package config

import (
	"html/template"
	"log"
	"os"
	"os/signal"
	"pawprintpublic/internal/diplomapdfs"
	"pawprintpublic/internal/mailer"
	"sync"
	"syscall"

	"github.com/alexedwards/scs/v2"
)

// AppConfig holds the application config
type AppConfig struct {
	UseCache      bool
	TemplateCache map[string]*template.Template
	InfoLog       *log.Logger
	ErrorLog      *log.Logger
	InProduction  bool
	Session       *scs.SessionManager
	Wait          *sync.WaitGroup
	Mailer        mailer.Mail
	ErrorChan     chan error
	ErrorChanDone chan bool
	TaskManager   *diplomapdfs.TaskManager
}

// Config is used for application startup to allow for easier testing of main.go
type Config struct {
	InProduction bool
	UseCache     bool
}

func (app *AppConfig) ListenForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	app.shutdown()
	os.Exit(0)
}

func (app *AppConfig) shutdown() {
	// perform any cleanup tasks
	app.InfoLog.Printf("running cleanup tasks...")

	// block until wait group is empty
	app.Wait.Wait()

	app.Mailer.DoneChan <- true
	app.ErrorChanDone <- true

	app.InfoLog.Println("closing channels and shutting down application...")
	close(app.Mailer.MailerChan)
	close(app.Mailer.ErrorChan)
	close(app.Mailer.DoneChan)
	close(app.ErrorChan)
	close(app.ErrorChanDone)
}

func (app *AppConfig) ListenForErrors() {
	for {
		select {
		case err := <-app.ErrorChan:
			// TODO - notify slack channel, write to db, etc
			app.ErrorLog.Println(err)
		case <-app.ErrorChanDone:
			return
		}
	}
}
