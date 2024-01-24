package dbstuff

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func GetDBFile(configHost string) (host, dbFilename string) {
	host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(configHost, "https://"), "http://"), ":443")
	dbFilename = host + ".sqlite"
	return host, dbFilename
}

func GetPool(ctx context.Context, configHost string) (pool *sqlitex.Pool, host string, err error) {
	host, fn := GetDBFile(configHost)
	pool, err = sqlitex.NewPool(fn, sqlitex.PoolOptions{})
	if err != nil {
		return nil, "", err
	}
	err = migrateDatabase(ctx, pool)
	if err != nil {
		return nil, "", err
	}
	return pool, host, nil
}

func migrateDatabase(ctx context.Context, pool *sqlitex.Pool) error {
	conn := pool.Get(ctx)
	defer pool.Put(conn)
	v, err := userVersion(conn)
	if err != nil {
		return fmt.Errorf("failed to get user_version of DB: %w", err)
	}
	for ; v < 1; v++ {
		var err error
		switch v {
		case 0:
			err = migrationToSchema0(ctx, pool)
		default:
			panic(fmt.Sprintf("I am confused. No matching schema migration found. %d", v))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type Resource struct {
	Id                int64
	Timestamp         time.Time
	ApiVersion        string
	Name              string
	Namespace         string
	CreationTimestamp time.Time
	Kind              string
	ResourceVersion   string
	Uid               string
	Json              string
}

func ResourceNewFromRow(stmt *sqlite.Stmt) Resource {
	var res Resource
	res.Id = stmt.ColumnInt64(0)
	res.Timestamp = time.UnixMicro(stmt.ColumnInt64(1))
	res.ApiVersion = stmt.ColumnText(2)
	res.Name = stmt.ColumnText(3)
	res.Namespace = stmt.ColumnText(4)
	res.CreationTimestamp = time.UnixMicro(stmt.ColumnInt64(5))
	res.Kind = stmt.ColumnText(6)
	res.ResourceVersion = stmt.ColumnText(7)
	res.Uid = stmt.ColumnText(8)
	res.Json = stmt.ColumnText(9)
	return res
}

func migrationToSchema0(ctx context.Context, pool *sqlitex.Pool) error {
	conn := pool.Get(ctx)
	defer pool.Put(conn)
	return sqlitex.ExecScript(conn, `
	CREATE TABLE res (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT DEFAULT(STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')),
		apiVersion TEXT,
		name TEXT,
		namespace TEXT,
		creationTimestamp TEXT,
		kind TEXT,
		resourceVersion TEXT,
		uid TEXT,
		json TEXT) STRICT;
		CREATE INDEX idx_apiversion ON res(apiVersion);
		CREATE INDEX idx_name ON res(name);
		CREATE INDEX idx_namespace ON res(namespace);
		CREATE INDEX idx_creationTimestamp ON res(creationTimestamp);
		CREATE INDEX idx_kind ON res(kind);
		CREATE INDEX idx_resourceVersion ON res(resourceVersion);
		CREATE INDEX idx_uid ON res(uid);
		PRAGMA user_version = 1;
		`)
}

func Query(ctx context.Context, pool *sqlitex.Pool, query string, opts *sqlitex.ExecOptions) error {
	conn := pool.Get(ctx)
	defer pool.Put(conn)
	return sqlitex.Execute(conn, query, opts)
}

func QueryText(ctx context.Context, pool *sqlitex.Pool, query string, queryArgs []any) (string, error) {
	var text string
	err := Query(ctx, pool, query, &sqlitex.ExecOptions{
		Args: queryArgs,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			t, err := sqlitex.ResultText(stmt)
			text = t
			if err != nil {
				return err
			}
			return nil
		},
	},
	)
	if err != nil {
		return text, err
	}
	return text, nil
}

func QueryInt(ctx context.Context, pool *sqlitex.Pool, query string, queryArgs []any) (int, error) {
	var ret int
	err := Query(ctx, pool, query, &sqlitex.ExecOptions{
		Args: queryArgs,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			i, err := sqlitex.ResultInt(stmt)
			ret = i
			if err != nil {
				return err
			}
			return nil
		},
	},
	)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func userVersion(conn *sqlite.Conn) (int32, error) {
	var version int32
	err := sqlitex.ExecuteTransient(conn, "PRAGMA user_version", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			version = stmt.ColumnInt32(0)
			return nil
		},
	})
	if err != nil {
		return 0, fmt.Errorf("get database user_version: %w", err)
	}
	return version, nil
}
