package postgres

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var PGHOST = os.Getenv("PGHOST")
var PGPORT = os.Getenv("PGPORT")
var PGUSER = os.Getenv("PGUSER")
var PGPASSWORD = os.Getenv("PGPASSWORD")
var PGDB = os.Getenv("PGDB")

var DBServer *gorm.DB = nil

func InitPostgres() {
	var err error

	if PGHOST == "" || PGPORT == "" || PGUSER == "" || PGPASSWORD == "" || PGDB == "" {
		fmt.Println("Postgres Database required environment variables are not set. Won't link to database.")
		return
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDB)

	DBServer, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Printf("Error connecting to PostgreSQL: %v\n", err)
		return
	}

	db, err := DBServer.DB()
	if err != nil {
		fmt.Printf("Error connecting to PostgreSQL: %v\n", err)
	}
	if err = db.Ping(); err != nil {
		fmt.Printf("Error pinging PostgreSQL: %v\n", err)
		return
	}

	fmt.Println("Successfully connected to PostgreSQL!")

	// db.AutoMigrate(&YourModel{})
	createTables()

	// test demo
	var count int
	DBServer.Raw("SELECT COUNT(*) FROM backups").Scan(&count)
	fmt.Printf("Count: %d of backup\n", count)
	DBServer.Raw("SELECT COUNT(*) FROM snapshots").Scan(&count)
	fmt.Printf("Count: %d of snapshot\n", count)
}

func createTables() {
	if DBServer == nil {
		return
	}
	if err := DBServer.AutoMigrate(&Backup{}, &Snapshot{}); err != nil {
		panic(err)
	}

	log.Println("Backup-Server tables created successfully")
}
