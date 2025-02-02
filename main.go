package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

var (
	host     = "localhost"
	port     = 5433
	user     = "claireli"
	password = ""
	dbname   = "sites_status"
)

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	password = os.Getenv("DB_PASSWORD")
}

var db *sql.DB

type Site struct {
	SiteName string `json:"site_name"`
	Status   string `json:"status"`
}

// ================== HANDLERS ==================
func indexHandler(w http.ResponseWriter, r *http.Request) {
	filePath := filepath.Join(".", "index.html")
	http.ServeFile(w, r, filePath)
}

func setStatusHandler(w http.ResponseWriter, r *http.Request) {
    siteName := r.FormValue("site-name")
    status := r.FormValue("status")

    _, err := db.Exec("INSERT INTO statuses (site_name, status) VALUES ($1, $2)", siteName, status)
    if err != nil {
        http.Error(w, "Database insert failed", http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/?success=true", http.StatusSeeOther)
}

func viewAllHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT site_name, status FROM statuses")
	if err != nil {
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sites := []Site{}
	for rows.Next() {
		var site Site
		if err := rows.Scan(&site.SiteName, &site.Status); err != nil {
			http.Error(w, "Error scanning database rows", http.StatusInternalServerError)
			return
		}
		sites = append(sites, site)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sites); err != nil {
		http.Error(w, "Error encoding response to JSON", http.StatusInternalServerError)
	}
}

func sendAllHandler(w http.ResponseWriter, r *http.Request) {
    if err := r.ParseForm(); err != nil {
        http.Error(w, "Invalid form data", http.StatusBadRequest)
        return
    }
    emails := r.Form["email[]"]

    if len(emails) == 0 {
        http.Error(w, "No email addresses provided", http.StatusBadRequest)
        return
    }

    // Fetch data from the database
    rows, err := db.Query("SELECT site_name, status FROM statuses")
    if err != nil {
        http.Error(w, "Database query failed", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var sites []Site
    for rows.Next() {
        var site Site
        if err := rows.Scan(&site.SiteName, &site.Status); err != nil {
            http.Error(w, "Error scanning database rows", http.StatusInternalServerError)
            return
        }
        sites = append(sites, site)
    }

    // Prepare the email content
    emailBody := "<h3>Sites Status</h3><ul>"
    for _, site := range sites {
        emailBody += fmt.Sprintf("<li>%s: %s</li>", site.SiteName, site.Status)
    }
    emailBody += "</ul>"

    // Send emails to all recipients
    from := mail.NewEmail("AutoDBData", os.Getenv("EMAIL"))
    subject := "Sites Status"
    plainTextContent := "Sites Status Report"
    htmlContent := emailBody

    client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))

    for _, email := range emails {
		to := mail.NewEmail("Recipient", email)
		message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
        go sendEmail(message, client, w, email)
    }
    http.Redirect(w, r, "/?success=true", http.StatusSeeOther)
}

func deleteAllHandler(w http.ResponseWriter, r *http.Request) {
    _, err := db.Exec("DELETE FROM statuses")
    if err != nil {
        http.Error(w, "Database delete failed", http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/?success=true", http.StatusSeeOther)
}

func main() {
	startBackend()

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))
	http.HandleFunc("/", indexHandler)
    http.HandleFunc("/status/set", setStatusHandler)
	http.HandleFunc("/view/all", viewAllHandler)
    http.HandleFunc("/send/all", sendAllHandler)
    http.HandleFunc("/delete/all", deleteAllHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	fmt.Println("Listening on port", port)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func startBackend() {
	loadEnv()
	pgConnStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	var err error
	db, err = sql.Open("postgres", pgConnStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Unable to ping database: %v", err)
	}
}

// ================== HELPER FUNCTIONS ==================
func sendEmail(message *mail.SGMailV3, client *sendgrid.Client, w http.ResponseWriter, email string) {
	response, err := client.Send(message)

    if err != nil || response.StatusCode >= 400 {
        log.Printf("Failed to send email: %v, Response: %v", err, response.Body)
        http.Error(w, "Email sending failed", http.StatusInternalServerError)
        return
    }
	log.Printf("Email sent successfully to %s", email)
}
