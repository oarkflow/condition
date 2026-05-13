package main

import runner "github.com/oarkflow/condition/examples/routing_file_backed_runner"

func main() {
	runner.Run("examples/routing_file_backed_permits/package.yaml", "examples/routing_file_backed_permits/request.json")
}
