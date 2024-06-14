package sqlcaller

import (
	"context"
	"database/sql/driver"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

func TestWithAnnotation(t *testing.T) {
	type args struct {
		driver string
		dsn    string
	}
	type testCase struct {
		name    string
		args    args
		wantErr bool
	}
	tests := []testCase{}
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		tests = append(tests, testCase{
			name: "mysql",
			args: args{
				driver: "mysql",
				dsn:    dsn,
			},
			wantErr: false,
		})
	}
	if dsn := os.Getenv("PG_DSN"); dsn != "" {
		tests = append(tests, testCase{
			name: "postgres",
			args: args{
				driver: "postgres",
				dsn:    dsn,
			},
			wantErr: false,
		})
	}
	if len(tests) == 0 {
		t.Fatal("no test cases found")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadline := time.Now().Add(time.Second * 30)
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()

			db, err := WithAnnotation(tt.args.driver, tt.args.dsn)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %+v, wantErr %+v", err, tt.wantErr)
				return
			}

			interval := time.Millisecond * 200
			for {
				err := db.PingContext(ctx)
				if err == nil {
					break
				}
				if err == driver.ErrBadConn {
					nextTick := time.Now().Add(interval)
					if nextTick.After(deadline) { // whether nextTick overs deadline
						t.Error("PingContext failed")
					}
					time.Sleep(interval)
					interval = time.Duration(2 * float64(interval))
					continue
				}
				if err != nil {
					t.Errorf("PingContext error = %+v", err)
					return
				}
			}

			if _, err := db.ExecContext(ctx, "select version()"); err != nil {
				t.Errorf("ExecContext error = %+v", err)
				return
			}
		})
	}
}
