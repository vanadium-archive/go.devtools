// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	gomail "gopkg.in/gomail.v1"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/cache"
)

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

func main() {
	ctx := tool.NewDefaultContext()

	root, err := project.JiriRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	emailUsername := os.Getenv("EMAIL_USERNAME")
	emailPassword := os.Getenv("EMAIL_PASSWORD")
	emails := strings.Split(os.Getenv("EMAILS"), " ")
	bucket := os.Getenv("NDA_BUCKET")

	// Download the NDA attachement from Google Cloud Storage
	attachment, err := cache.StoreGoogleStorageFile(ctx, root, bucket, "google-agreement.pdf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// Use the Google Apps SMTP relay to send the welcome email, this has been
	// pre-configured to allow authentiated v.io accounts to send mail.
	mailer := gomail.NewMailer("smtp-relay.gmail.com", emailUsername, emailPassword, 587)
	messages := []string{}
	for _, email := range emails {
		if strings.TrimSpace(email) == "" {
			continue
		}

		if err := sendWelcomeEmail(ctx, mailer, email, attachment); err != nil {
			messages = append(messages, err.Error())
		}
	}

	// Log any errors from sending the email messages.
	if len(messages) > 0 {
		message := strings.Join(messages, "\n\n")
		err := errors.New(message)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func sendWelcomeEmail(ctx *tool.Context, mailer *gomail.Mailer, email string, attachment string) error {
	// Read message data from Google Storage bucket and parse it.
	var m message
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	if err := ctx.Run().CommandWithOpts(opts, "gsutil", "-q", "cat", mailerTemplateFile); err != nil {
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
