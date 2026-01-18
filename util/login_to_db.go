package util

import (
	"database/sql"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func LoginToDB() *sql.DB {
	err := godotenv.Load(".env")
	CheckErr(err)
	db, err := sql.Open("postgres", "user="+os.Getenv("ENVUSER")+" password="+os.Getenv("ENVPASS")+" dbname=UniGoDB sslmode=disable")
	CheckErr(err)
	return db
}