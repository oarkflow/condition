package condition

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStoreOption func(*sqliteStoreConfig)

type sqliteStoreConfig struct {
	driverName string
}

func WithSQLiteDriverName(driverName string) SQLiteStoreOption {
	return func(c *sqliteStoreConfig) {
		if driverName != "" {
			c.driverName = driverName
		}
	}
}

type SQLiteDecisionStore struct {
	mu *sync.Mutex
	db *sql.DB
}

func NewSQLiteDecisionStore(path string, opts ...SQLiteStoreOption) (*SQLiteDecisionStore, error) {
	cfg := sqliteStoreConfig{driverName: "sqlite"}
	for _, opt := range opts {
		opt(&cfg)
	}
	if path == "" {
		return nil, errors.New("condition: sqlite store path is required")
	}
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "." && dir != "" {
			// SQLite creates the database file but not parent directories.
			// Keep directory creation with the caller; this error is clearer.
		}
	}
	db, err := sql.Open(cfg.driverName, path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteDecisionStore{db: db, mu: new(sync.Mutex)}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteDecisionStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteDecisionStore) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS decision_packages (
			key TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			environment TEXT NOT NULL,
			digest TEXT NOT NULL,
			published_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_decision_packages_latest ON decision_packages(name, environment, published_at)`,
		`CREATE TABLE IF NOT EXISTS audit_envelopes (
			id TEXT PRIMARY KEY,
			sequence INTEGER NOT NULL UNIQUE,
			timestamp TEXT NOT NULL,
			previous_hash TEXT,
			payload_hash TEXT NOT NULL,
			chain_hash TEXT NOT NULL,
			signature TEXT,
			record_payload TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS simulations (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteDecisionStore) SavePackage(ctx context.Context, pkg DecisionPackage, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	version := pkg.Version
	env := pkg.Environment
	key := packageKey(pkg.Name, version, env)
	publishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO decision_packages(key,name,version,environment,digest,published_at,payload) VALUES(?,?,?,?,?,?,?)`,
		key, pkg.Name, version, env, digest, publishedAt, string(payload))
	return err
}

func (s *SQLiteDecisionStore) ListPackages(ctx context.Context) ([]PackageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload,digest,published_at FROM decision_packages ORDER BY name, environment, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PackageRecord
	for rows.Next() {
		record, err := scanPackageRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteDecisionStore) GetPackage(ctx context.Context, name, version, environment string) (PackageRecord, bool, error) {
	var row *sql.Row
	if version != "" {
		row = s.db.QueryRowContext(ctx, `SELECT payload,digest,published_at FROM decision_packages WHERE key = ?`, packageKey(name, version, environment))
	} else {
		row = s.db.QueryRowContext(ctx, `SELECT payload,digest,published_at FROM decision_packages WHERE name = ? AND environment = ? ORDER BY published_at DESC LIMIT 1`, name, environment)
	}
	record, err := scanPackageRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return PackageRecord{}, false, nil
	}
	if err != nil {
		return PackageRecord{}, false, err
	}
	return record, true, nil
}

type packageScanner interface {
	Scan(dest ...any) error
}

func scanPackageRecord(scanner packageScanner) (PackageRecord, error) {
	var payload, digest, publishedAt string
	if err := scanner.Scan(&payload, &digest, &publishedAt); err != nil {
		return PackageRecord{}, err
	}
	var pkg DecisionPackage
	if err := json.Unmarshal([]byte(payload), &pkg); err != nil {
		return PackageRecord{}, err
	}
	return PackageRecord{Package: pkg, Digest: digest, PublishedAt: publishedAt}, nil
}

func (s *SQLiteDecisionStore) SaveAudit(ctx context.Context, audit AuditRecord) (string, error) {
	envelope, err := s.SaveAuditEnvelope(ctx, audit)
	if err != nil {
		return "", err
	}
	return envelope.ID, nil
}

func (s *SQLiteDecisionStore) SaveAuditEnvelope(ctx context.Context, audit AuditRecord) (AuditEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return AuditEnvelope{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuditEnvelope{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var prevSeq uint64
	var prevHash string
	row := tx.QueryRowContext(ctx, `SELECT sequence, chain_hash FROM audit_envelopes ORDER BY sequence DESC LIMIT 1`)
	switch err := row.Scan(&prevSeq, &prevHash); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return AuditEnvelope{}, err
	}
	sequence := prevSeq + 1
	id := "audit_" + strconvFormatInt(int64(sequence))
	payloadHash, err := auditPayloadHash(audit)
	if err != nil {
		return AuditEnvelope{}, err
	}
	envelope := AuditEnvelope{
		ID:           id,
		Sequence:     sequence,
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		PreviousHash: prevHash,
		PayloadHash:  payloadHash,
		Record:       audit,
	}
	envelope.ChainHash = auditChainHash(envelope.Sequence, envelope.Timestamp, envelope.ID, envelope.PreviousHash, envelope.PayloadHash)
	recordPayload, err := json.Marshal(audit)
	if err != nil {
		return AuditEnvelope{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_envelopes(id,sequence,timestamp,previous_hash,payload_hash,chain_hash,signature,record_payload) VALUES(?,?,?,?,?,?,?,?)`,
		envelope.ID, envelope.Sequence, envelope.Timestamp, envelope.PreviousHash, envelope.PayloadHash, envelope.ChainHash, envelope.Signature, string(recordPayload))
	if err != nil {
		return AuditEnvelope{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuditEnvelope{}, err
	}
	return envelope, nil
}

func (s *SQLiteDecisionStore) ListAudits(ctx context.Context) ([]AuditRecord, error) {
	envelopes, err := s.ListAuditEnvelopes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AuditRecord, 0, len(envelopes))
	for _, envelope := range envelopes {
		out = append(out, envelope.Record)
	}
	return out, nil
}

func (s *SQLiteDecisionStore) ListAuditEnvelopes(ctx context.Context) ([]AuditEnvelope, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,sequence,timestamp,previous_hash,payload_hash,chain_hash,signature,record_payload FROM audit_envelopes ORDER BY sequence ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEnvelope
	for rows.Next() {
		envelope, err := scanAuditEnvelope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, envelope)
	}
	return out, rows.Err()
}

func (s *SQLiteDecisionStore) GetAudit(ctx context.Context, id string) (AuditRecord, bool, error) {
	envelope, ok, err := s.GetAuditEnvelope(ctx, id)
	if err != nil || !ok {
		return AuditRecord{}, ok, err
	}
	return envelope.Record, true, nil
}

func (s *SQLiteDecisionStore) GetAuditEnvelope(ctx context.Context, id string) (AuditEnvelope, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,sequence,timestamp,previous_hash,payload_hash,chain_hash,signature,record_payload FROM audit_envelopes WHERE id = ?`, id)
	envelope, err := scanAuditEnvelope(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AuditEnvelope{}, false, nil
	}
	if err != nil {
		return AuditEnvelope{}, false, err
	}
	return envelope, true, nil
}

type auditScanner interface {
	Scan(dest ...any) error
}

func scanAuditEnvelope(scanner auditScanner) (AuditEnvelope, error) {
	var envelope AuditEnvelope
	var recordPayload string
	if err := scanner.Scan(&envelope.ID, &envelope.Sequence, &envelope.Timestamp, &envelope.PreviousHash, &envelope.PayloadHash, &envelope.ChainHash, &envelope.Signature, &recordPayload); err != nil {
		return AuditEnvelope{}, err
	}
	if err := json.Unmarshal([]byte(recordPayload), &envelope.Record); err != nil {
		return AuditEnvelope{}, err
	}
	return envelope, nil
}

func (s *SQLiteDecisionStore) SaveSimulation(ctx context.Context, sim SimulationResult) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(sim)
	if err != nil {
		return "", err
	}
	id := "sim_" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	_, err = s.db.ExecContext(ctx, `INSERT INTO simulations(id,created_at,payload) VALUES(?,?,?)`, id, time.Now().UTC().Format(time.RFC3339Nano), string(payload))
	return id, err
}
