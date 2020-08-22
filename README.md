![CI](https://github.com/aereal/go-sql-caller-annotation/workflows/CI/badge.svg?branch=main)
[![PkgGoDev](https://pkg.go.dev/badge/aereal/go-sql-caller-annotation)](https://pkg.go.dev/github.com/aereal/go-sql-caller-annotation)

# go-sql-caller-annotation

Provides an new `sql.*DB` connection that injects caller information to the query

```sh
go get github.com/aereal/go-sql-caller-annotation
```

## Usage

```go
package main

import (
  "github.com/aereal/go-sql-caller-annotation/sqlcaller"
)

func main() {
  db, _ := sqlcaller.WithAnnotation("mysql", "..." /* DSN */)
  db.Exec("SELECT version()") // runs `/* main.main (/path/to/file.go:9) */ SELECT version()` query
}
```

## License

See LICENSE file.
