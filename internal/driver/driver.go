package driver

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

// DB holds the database connection pool
type DB struct {
	SQL *sql.DB
}

var dbConn = &DB{}

const maxOpenDbConn = 10
const maxIdleDbConn = 5
const maxDbLifetime = 5 * time.Minute

// ConnectSQL creates database pool for Postgres
func ConnectSQL(dsn string) (*DB, error) {
	db, err := NewDatabase(dsn)
	if err != nil {
		panic(err)
	}

	db.SetMaxOpenConns(maxOpenDbConn)
	db.SetMaxIdleConns(maxIdleDbConn)
	db.SetConnMaxLifetime(maxDbLifetime)

	dbConn.SQL = db

	err = testDB(db)
	if err != nil {
		return nil, err
	}
	return dbConn, nil
}

// testDB tries to ping the database
func testDB(d *sql.DB) error {
	err := d.Ping()
	if err != nil {
		return err
	}
	return nil
}

// NewDatabase creates a new database for the application
func NewDatabase(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// package driver

// import (
// 	"database/sql"
// 	"time"

// 	_ "github.com/glebarez/go-sqlite"
// )

// // DB holds the database connection pool
// type DB struct {
// 	SQL *sql.DB
// }

// var dbConn = &DB{}

// const maxOpenDbConn = 10
// const maxIdleDbConn = 5
// const maxDbLifetime = 5 * time.Minute

// // ConnectSQL creates database pool for Postgres
// func ConnectSQL() (*DB, error) {
// 	db, err := NewDatabase()
// 	if err != nil {
// 		panic(err)
// 	}

// 	db.SetMaxOpenConns(maxOpenDbConn)
// 	db.SetMaxIdleConns(maxIdleDbConn)
// 	db.SetConnMaxLifetime(maxDbLifetime)

// 	dbConn.SQL = db

// 	err = testDB(db)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return dbConn, nil
// }

// // testDB tries to ping the database
// func testDB(d *sql.DB) error {
// 	err := d.Ping()
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// // NewDatabase creates a new database for the application
// func NewDatabase() (*sql.DB, error) {
// 	db, err := sql.Open("sqlite", "pawprint.db")
// 	if err != nil {
// 		return nil, err
// 	}

// 	if err = db.Ping(); err != nil {
// 		return nil, err
// 	}

// 	return db, nil
// }
