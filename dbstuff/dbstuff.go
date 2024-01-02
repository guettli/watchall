package dbstuff

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func GetDBFile(configHost string) (host, dbFilename string) {
	host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(configHost, "https://"), "http://"), ":443")
	dbFilename = host + ".sqlite"
	return host, dbFilename
}

func GetDB(configHost string) (db *sql.DB, host string, err error) {
	host, fn := GetDBFile(configHost)
	db, err = sql.Open("sqlite", fn)
	if err != nil {
		return nil, "", err
	}
	err = migrateDatabase(db)
	if err != nil {
		return nil, "", err
	}
	return db, host, nil
}

func migrateDatabase(db *sql.DB) error {
	v := 0
	err := db.QueryRow("pragma user_version").Scan(&v)
	if err != nil {
		return err
	}
	for ; v < 1; v++ {
		var err error
		switch v {
		case 0:
			err = migrationToSchema0(db)
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
	ApiVersion        string
	Name              string
	Namespace         string
	CreationTimestamp time.Time
	Kind              string
	ResourceVersion   string
	Uid               string
	Json              string
}

type RowScanner interface {
	Scan(dest ...any) error
}

func ResourceNewFromRow(scanner RowScanner) (Resource, error) {
	var res Resource
	var timestamp string
	err := scanner.Scan(&res.Id, &res.ApiVersion, &res.Name, &res.Namespace, &timestamp, &res.Kind,
		&res.ResourceVersion, &res.Uid, &res.Json)
	if err != nil {
		return res, err
	}
	res.CreationTimestamp, err = time.Parse("2006-01-02T15:04:05Z", timestamp)
	return res, err
}

func migrationToSchema0(db *sql.DB) error {
	_, err := db.Exec(`
	BEGIN;
	CREATE TABLE res (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		apiVersion TEXT,
		name TEXT,
		namespace TEXT,
		creationTimestamp TEXT,
		kind TEXT,
		resourceVersion TEXT,
		uid TEXT,
		json TEXT);
		CREATE INDEX idx_apiversion ON res(apiVersion);
		CREATE INDEX idx_name ON res(name);
		CREATE INDEX idx_namespace ON res(namespace);
		CREATE INDEX idx_creationTimestamp ON res(creationTimestamp);
		CREATE INDEX idx_kind ON res(kind);
		CREATE INDEX idx_resourceVersion ON res(resourceVersion);
		CREATE INDEX idx_uid ON res(uid);
		PRAGMA user_version = 1;
		COMMIT;
		`)
	return err
}
