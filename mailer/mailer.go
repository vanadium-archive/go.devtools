// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go . -help

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	gomail "gopkg.in/gomail.v1"
	"v.io/jiri"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/cache"
	"v.io/x/lib/cmdline"
)

var cmdMailer = &cmdline.Command{
	Runner: jiri.RunnerFunc(runMailer),
	Name:   "mailer",
	Short:  "sends vanadium welcome email",
	Long: `
Command mailer sends the vanadium welcome email, including the NDA as an
attachment.  The email is sent via smtp-relay.gmail.com.

Due to legacy reasons(?), the configuration is via environment variables:
	EMAIL_USERNAME: Sender email username.
	EMAIL_PASSWORD: Sender email password.
	EMAILS:         Space-separated list of recipient email addresses.
	NDA_BUCKET:     Google-storage bucket that contains the NDA.
`,
}

func main() {
	cmdline.Main(cmdMailer)
}

const mailerTemplateFile = "gs://vanadium-mailer/template.json"

type link struct {
	Url  string
	Name string
}

type message struct {
	Lines []string
	Links map[string]link
}

func (m message) plain() string {
	result := strings.Join(m.Lines, "\n\n")

	for key, link := range m.Links {
		result = strings.Replace(result, key, fmt.Sprintf("%v (%v)", link.Name, link.Url), -1)
	}
	return result
}

func (m message) html() string {
	result := ""
	for _, line := range m.Lines {
		result += fmt.Sprintf("<p>\n%v\n</p>\n", line)
	}

	for key, link := range m.Links {
		result = strings.Replace(result, key, fmt.Sprintf("<a href=%q>%v</a>", link.Url, link.Name), -1)
	}
	return result
}

func runMailer(jirix *jiri.X, args []string) error {
	emailUsername := jirix.Env()["EMAIL_USERNAME"]
	emailPassword := jirix.Env()["EMAIL_PASSWORD"]
	emails := strings.Split(jirix.Env()["EMAILS"], " ")
	bucket := jirix.Env()["NDA_BUCKET"]

	fmt.Fprintf(os.Stderr, "ENV: %v\n", jirix.Env())

	// Download the NDA attachement from Google Cloud Storage
	attachment, err := cache.StoreGoogleStorageFile(jirix, jirix.Root, bucket, "google-agreement.pdf")
	if err != nil {
		return err
	}

	// Use the Google Apps SMTP relay to send the welcome email, this has been
	// pre-configured to allow authentiated v.io accounts to send mail.
	mailer := gomail.NewMailer("smtp-relay.gmail.com", emailUsername, emailPassword, 587)
	messages := []string{}
	for _, email := range emails {
		if strings.TrimSpace(email) == "" {
			continue
		}

		if err := sendWelcomeEmail(jirix.Context, mailer, email, attachment); err != nil {
			messages = append(messages, err.Error())
		}
	}

	// Return errors from sending the email messages.
	if len(messages) > 0 {
		return errors.New(strings.Join(messages, "\n\n"))
	}
	return nil
}

func sendWelcomeEmail(ctx *tool.Context, mailer *gomail.Mailer, email string, attachment string) error {
	// Read message data from Google Storage bucket and parse it.
	var m message
	var out bytes.Buffer
	if err := ctx.NewSeq().Capture(&out, nil).Last("gsutil", "-q", "cat", mailerTemplateFile); err != nil {
		return err
	}
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		return fmt.Errorf("Unmarshal() failed: %v", err)
	}

	message := gomail.NewMessage()
	message.SetHeader("From", "Vanadium Team <welcome@v.io>")
	message.SetHeader("To", email)
	message.SetHeader("Subject", "Vanadium early access activated")
	message.SetBody("text/plain", m.plain())
	message.AddAlternative("text/html", m.html())

	file, err := gomail.OpenFile(attachment)
	if err != nil {
		return fmt.Errorf("OpenFile(%v) failed: %v", attachment, err)
	}

	message.Attach(file)

	if err := mailer.Send(message); err != nil {
		return fmt.Errorf("Send(%v) failed: %v", message, err)
	}

	return nil
}
