package dbrepo

import (
	"database/sql"
	"pawprintpublic/internal/config"
	"pawprintpublic/internal/repository"
)

// type postgresDBRepo struct {
// 	App *config.AppConfig
// 	DB  *sql.DB
// }

type sqliteDBRepo struct {
	App *config.AppConfig
	DB  *sql.DB
}

type testDBRepo struct {
	App *config.AppConfig
	DB  *sql.DB
}

func NewPostgresRepo(conn *sql.DB, a *config.AppConfig) repository.DatabaseRepo {
	return &sqliteDBRepo{
		App: a,
		DB:  conn,
	}
}

func NewTestingsRepo(a *config.AppConfig) repository.DatabaseRepo {
	return &testDBRepo{
		App: a,
	}
}
