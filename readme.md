# Cougar Paw Prints

This is the repository for the Collin College Paw Print Application, used by the Graduation Department during the diploma printing process.

# Overview

Cougar Paw Prints is a Go-based web application designed to streamline the diploma printing process. The application handles diploma data, provides a web interface for administrative tasks.

### Built with:

- Go version 1.22.x

## Dependencies:

- [chi router](https://github.com/go-chi/chi)
- [alex edwards SCS](https://github.com/alexedwards/scs/v2) session management
- [nosurf](https://github.com/justinas/nosurf)
- [go-sqlite](https://github.com/glebarez/go-sqlite)
- [simple mail](https://github.com/xhit/go-simple-mail/v2)
- [Go validator](https://github.com/asaskevich/govalidator)
- [gofpdf](https://github.com/phpdave11/gofpdf)
- [pdfcpu](https://github.com/pdfcpu/pdfcpu)
- [excelize](https://github.com/qax-os/excelize)
- [crypto](https://pkg.go.dev/golang.org/x/crypto)

## Getting Started

To run the application, follow these steps:

### Prerequisites

Ensure you have Go 1.22.x installed and Docker (if you're running it in a container).

### Running the Application Locally

1. Download Modules

   Ensure all Go modules are downloaded and dependencies are resolved:

   ```
   go mod tidy
   go mod download
   ```

2. Update mailer config in main.go to use your preferred service.

   ```
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
   ```

3. Run the Application

   You can run the application locally with the following command:

   ```
   go run ./cmd/web
   ```

   This will start the application on http://localhost:8080.

### Running the Application in a Docker Container

1. Build the Docker Image

   Use docker buildx to build the Docker image:

   ```
   docker buildx build --platform linux/amd64 -t pawprintpublic:latest --load .
   ```

2. Run the Docker Container

   Run the container and expose the application on port 8080:

   ```
   docker run -p 8080:8080 pawprintpublic:latest
   ```

## License

This project is licensed under the [MIT License]().
