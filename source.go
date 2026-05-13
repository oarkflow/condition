package condition

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Source loads raw bytes from a boundary such as a file, HTTP endpoint, or
// embedded string. Decoders decide what those bytes mean.
type Source interface {
	Load(context.Context) ([]byte, error)
}

// Watcher is an optional extension for sources that can signal reloads.
type Watcher interface {
	Watch(context.Context) (<-chan struct{}, <-chan error)
}

// Decoder turns source bytes into a typed value.
type Decoder[T any] interface {
	Decode([]byte) (T, error)
}

type DecoderFunc[T any] func([]byte) (T, error)

func (f DecoderFunc[T]) Decode(data []byte) (T, error) { return f(data) }

// BytesSource is useful for tests and embedded rule definitions.
type BytesSource []byte

func (s BytesSource) Load(context.Context) ([]byte, error) {
	out := make([]byte, len(s))
	copy(out, s)
	return out, nil
}

// StringSource is useful for tests and embedded rule definitions.
type StringSource string

func (s StringSource) Load(context.Context) ([]byte, error) { return []byte(s), nil }

// FileSource loads bytes from a local file.
type FileSource struct {
	Path string
}

func (s FileSource) Load(context.Context) ([]byte, error) {
	if s.Path == "" {
		return nil, errors.New("condition: file source path is required")
	}
	return os.ReadFile(s.Path)
}

// HTTPSource loads bytes from an HTTP endpoint using the caller-provided client.
type HTTPSource struct {
	URL    string
	Method string
	Header http.Header
	Client *http.Client
}

func (s HTTPSource) Load(ctx context.Context) ([]byte, error) {
	if s.URL == "" {
		return nil, errors.New("condition: http source url is required")
	}
	method := s.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, s.URL, nil)
	if err != nil {
		return nil, err
	}
	for key, values := range s.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("condition: http source returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func LoadValue[T any](ctx context.Context, source Source, decoder Decoder[T]) (T, error) {
	var zero T
	if source == nil {
		return zero, errors.New("condition: source is nil")
	}
	if decoder == nil {
		return zero, errors.New("condition: decoder is nil")
	}
	data, err := source.Load(ctx)
	if err != nil {
		return zero, err
	}
	return decoder.Decode(data)
}

func LoadRuleSet(ctx context.Context, source Source, decoder Decoder[RuleSet]) (RuleSet, error) {
	return LoadValue(ctx, source, decoder)
}

func LoadRuleSets(ctx context.Context, source Source, decoder Decoder[[]RuleSet]) ([]RuleSet, error) {
	return LoadValue(ctx, source, decoder)
}

func LoadFacts(ctx context.Context, source Source, decoder Decoder[MapFacts]) (Facts, error) {
	return LoadValue(ctx, source, decoder)
}

func LoadExpression(ctx context.Context, source Source, decoder Decoder[string], opts ...Option) (*Expression, error) {
	expr, err := LoadValue(ctx, source, decoder)
	if err != nil {
		return nil, err
	}
	return Compile(expr, opts...)
}

func JSONDecoder[T any]() Decoder[T] {
	return DecoderFunc[T](func(data []byte) (T, error) {
		var out T
		err := json.Unmarshal(data, &out)
		return out, err
	})
}

func StringDecoder() Decoder[string] {
	return DecoderFunc[string](func(data []byte) (string, error) {
		return strings.TrimSpace(string(data)), nil
	})
}

func CSVFactsDecoder() Decoder[MapFacts] {
	return DecoderFunc[MapFacts](func(data []byte) (MapFacts, error) {
		rows, err := decodeCSVRows(data)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return MapFacts{}, nil
		}
		return rows[0], nil
	})
}

func CSVFactsListDecoder() Decoder[[]MapFacts] {
	return DecoderFunc[[]MapFacts](decodeCSVRows)
}

func decodeCSVRows(data []byte) ([]MapFacts, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	headers := records[0]
	out := make([]MapFacts, 0, len(records)-1)
	for rowIndex, record := range records[1:] {
		if len(record) != len(headers) {
			return nil, fmt.Errorf("condition: csv row %d has %d fields, want %d", rowIndex+2, len(record), len(headers))
		}
		facts := MapFacts{}
		for i, header := range headers {
			path := strings.TrimSpace(header)
			if path == "" {
				continue
			}
			if err := setFactPath(facts, strings.Split(path, "."), parseCSVValue(record[i])); err != nil {
				return nil, fmt.Errorf("condition: csv row %d column %q: %w", rowIndex+2, path, err)
			}
		}
		out = append(out, facts)
	}
	return out, nil
}

func parseCSVValue(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format(time.RFC3339)
	}
	return raw
}

func setFactPath(root MapFacts, parts []string, value any) error {
	if len(parts) == 0 {
		return errors.New("empty path")
	}
	var cur any = map[string]any(root)
	for i, part := range parts {
		if part == "" {
			return errors.New("empty path segment")
		}
		last := i == len(parts)-1
		nextIsIndex := !last && isIndex(parts[i+1])
		switch node := cur.(type) {
		case map[string]any:
			if last {
				node[part] = value
				return nil
			}
			child, ok := node[part]
			if !ok {
				if nextIsIndex {
					child = []any{}
				} else {
					child = map[string]any{}
				}
				node[part] = child
			}
			cur = child
		case []any:
			index, err := parseIndex(part)
			if err != nil {
				return err
			}
			node = growSlice(node, index+1)
			if last {
				node[index] = value
			} else if node[index] == nil {
				if nextIsIndex {
					node[index] = []any{}
				} else {
					node[index] = map[string]any{}
				}
			}
			cur = node[index]
			if i == 0 {
				return errors.New("root path cannot start with an index")
			}
			if err := assignSlice(root, parts[:i], node); err != nil {
				return err
			}
			if last {
				return nil
			}
		default:
			return fmt.Errorf("cannot descend into %T", cur)
		}
	}
	return nil
}

func assignSlice(root MapFacts, parts []string, value []any) error {
	if len(parts) == 0 {
		return errors.New("root path cannot be a slice")
	}
	var cur any = map[string]any(root)
	for i, part := range parts {
		last := i == len(parts)-1
		switch node := cur.(type) {
		case map[string]any:
			if last {
				node[part] = value
				return nil
			}
			cur = node[part]
		case []any:
			index, err := parseIndex(part)
			if err != nil {
				return err
			}
			if index >= len(node) {
				return fmt.Errorf("slice index %d out of range", index)
			}
			if last {
				node[index] = value
				return nil
			}
			cur = node[index]
		default:
			return fmt.Errorf("cannot descend into %T", cur)
		}
	}
	return nil
}

func isIndex(s string) bool {
	_, err := parseIndex(s)
	return err == nil
}

func parseIndex(s string) (int, error) {
	index, err := strconv.Atoi(s)
	if err != nil || index < 0 {
		return 0, fmt.Errorf("invalid slice index %q", s)
	}
	return index, nil
}

func growSlice(in []any, size int) []any {
	if len(in) >= size {
		return in
	}
	out := make([]any, size)
	copy(out, in)
	return out
}

// RowSource is an adapter point for CSV, SQL, or other tabular sources.
type RowSource interface {
	Rows(context.Context) (Rows, error)
}

type Rows interface {
	Columns() []string
	Next() (map[string]any, bool, error)
	Close() error
}

type SQLSource struct {
	DB    *sql.DB
	Query string
	Args  []any
}

func (s SQLSource) Rows(ctx context.Context) (Rows, error) {
	if s.DB == nil {
		return nil, errors.New("condition: sql source db is nil")
	}
	if strings.TrimSpace(s.Query) == "" {
		return nil, errors.New("condition: sql source query is required")
	}
	rows, err := s.DB.QueryContext(ctx, s.Query, s.Args...)
	if err != nil {
		return nil, err
	}
	columns, err := rows.Columns()
	if err != nil {
		rows.Close()
		return nil, err
	}
	return &sqlRows{rows: rows, columns: columns}, nil
}

func NewSQLRows(rows *sql.Rows) (Rows, error) {
	if rows == nil {
		return nil, errors.New("condition: sql rows is nil")
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows, columns: columns}, nil
}

type sqlRows struct {
	rows    *sql.Rows
	columns []string
}

func (r *sqlRows) Columns() []string { return r.columns }

func (r *sqlRows) Next() (map[string]any, bool, error) {
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	values := make([]any, len(r.columns))
	dest := make([]any, len(r.columns))
	for i := range values {
		dest[i] = &values[i]
	}
	if err := r.rows.Scan(dest...); err != nil {
		return nil, false, err
	}
	row := make(map[string]any, len(r.columns))
	for i, column := range r.columns {
		value := values[i]
		if b, ok := value.([]byte); ok {
			value = string(b)
		}
		row[column] = value
	}
	return row, true, nil
}

func (r *sqlRows) Close() error { return r.rows.Close() }

func LoadRowFacts(ctx context.Context, source RowSource) ([]MapFacts, error) {
	if source == nil {
		return nil, errors.New("condition: row source is nil")
	}
	rows, err := source.Rows(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MapFacts
	for {
		row, ok, err := rows.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		facts := MapFacts{}
		for key, value := range row {
			if err := setFactPath(facts, strings.Split(key, "."), value); err != nil {
				return nil, fmt.Errorf("condition: row column %q: %w", key, err)
			}
		}
		out = append(out, facts)
	}
}

// FactSource can resolve facts lazily from caches, databases, or services.
type FactSource interface {
	GetFact(context.Context, string) (any, bool, error)
}

type DynamicFacts struct {
	Context context.Context
	Source  FactSource
	OnError func(error)
}

func (f DynamicFacts) Get(path string) (any, bool) {
	if f.Source == nil {
		return nil, false
	}
	ctx := f.Context
	if ctx == nil {
		ctx = context.Background()
	}
	value, ok, err := f.Source.GetFact(ctx, path)
	if err != nil {
		if f.OnError != nil {
			f.OnError(err)
		}
		return nil, false
	}
	return value, ok
}
