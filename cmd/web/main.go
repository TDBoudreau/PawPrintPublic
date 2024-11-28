package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"pawprintpublic/internal/config"
	"pawprintpublic/internal/diplomapdfs"
	"pawprintpublic/internal/driver"
	"pawprintpublic/internal/handlers"
	"pawprintpublic/internal/helpers"
	"pawprintpublic/internal/mailer"
	"pawprintpublic/internal/models"
	"pawprintpublic/internal/render"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
)

const portNumber = ":8080"

var app config.AppConfig
var session *scs.SessionManager
var infoLog *log.Logger
var errorLog *log.Logger

// main is the main application function
func main() {
	fmt.Println("Basic stdout test")
	// Parse flags
	inProduction := flag.Bool("production", true, "Application is in production")
	useCache := flag.Bool("cache", false, "Use template cache")

	flag.Parse()

	cfg := config.Config{
		InProduction: *inProduction,
		UseCache:     *useCache,
	}

	db, err := run(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err = db.SQL.Close()
		if err != nil {
			app.ErrorLog.Fatalln(err)
		}
	}()

	fmt.Println("Starting mail listener...")
	// Listen for mail, shutdown signal, and error channel signals
	go app.Mailer.ListenForMail()
	go app.ListenForShutdown()
	go app.ListenForErrors()

	fmt.Printf("Starting application on port %s\n", portNumber)

	srv := &http.Server{
		Addr:    portNumber,
		Handler: routes(&app),
	}

	err = srv.ListenAndServe()
	log.Fatal(err)
}

func run(cfg config.Config) (*driver.DB, error) {
	// Register types for session management
	// gob.Register(models.Reservation{})
	gob.Register(models.User{})
	// gob.Register(models.Room{})
	// gob.Register(models.Restriction{})
	gob.Register(map[string]int{})

	// Initialize Logger
	infoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.InfoLog = infoLog

	errorLog = log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)
	app.ErrorLog = errorLog

	// app.InfoLog.Println("Logger initialized - test message")
	// app.ErrorLog.Println("Error logger initialized - test message")

	// Initialize Session
	session = scs.New()
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = cfg.InProduction
	app.Session = session

	// Initialize AppConfig fields
	app.InProduction = cfg.InProduction
	app.UseCache = cfg.UseCache
	app.Wait = &sync.WaitGroup{}
	app.ErrorChan = make(chan error)
	app.ErrorChanDone = make(chan bool)
	app.TaskManager = diplomapdfs.NewTaskManager()

	// Connect to database
	log.Println("Connecting to database...")
	db, err := driver.ConnectSQL()
	if err != nil {
		log.Println("Cannot connect to database! Dying...")
		return nil, err
	}
	log.Println("Connected to database!")

	// Initialize Template Cache
	tc, err := render.CreateTemplateCache()
	if err != nil {
		log.Println("Cannot create template cache")
		return nil, err
	}
	app.TemplateCache = tc

	// Initialize Mailer Configuration
	mailerConfig := mailer.MailConfig{
		Domain:      "localhost",
		Host:        "localhost",
		Port:        1025,
		Username:    "",
		Password:    "",
		Encryption:  "none",
		FromAddress: "info@mycompany.com",
		FromName:    "Info",
		Wait:        app.Wait,
		InfoLog:     app.InfoLog,
		ErrorLog:    app.ErrorLog,
	}

	// Initialize Mailer
	app.Mailer = mailer.CreateMail(mailerConfig)

	// Initialize Handlers, Renderer, and Helpers
	repo := handlers.NewRepo(&app, db)
	handlers.NewHandlers(repo)

	repo.StartCleanupJob()

	render.NewRenderer(&app)
	helpers.NewHelpers(&app)

	return db, nil
}
