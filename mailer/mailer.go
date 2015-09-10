// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	gomail "gopkg.in/gomail.v1"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/cache"
)

type link struct {
	url  string
	name string
}

type message struct {
	lines []string
	links map[string]link
}

func (m message) plain() string {
	result := strings.Join(m.lines, "\n\n")

	for key, link := range m.links {
		result = strings.Replace(result, key, fmt.Sprintf("%v (%v)", link.name, link.url), -1)
	}
	return result
}

func (m message) html() string {
	result := ""
	for _, line := range m.lines {
		result += fmt.Sprintf("<p>\n%v\n</p>\n", line)
	}

	for key, link := range m.links {
		result = strings.Replace(result, key, fmt.Sprintf("<a href=%q>%v</a>", link.url, link.name), -1)
	}
	return result
}

var m = message{
	lines: []string{
		"Welcome to the Vanadium project. Your early access has been activated.",
		"What now?",
		"To understand a bit more about why we are building Vanadium and what we are trying to achieve with this early access program, read our [[0]].",
		"The project’s website [[1]], includes information about Vanadium including key concepts, tutorials, and how to access our codebase.",
		"Sign up for our mailing list, [[2]], and send any questions or feedback there.",
		"As mentioned earlier, please keep this project confidential until it is publicly released. If there is anyone else that you think would benefit from access to this project, send them [[3]].",
		"Thanks for participating,",
		"The Vanadium Team",
	},
	links: map[string]link{
		"[[0]]": link{
			url:  "https://v.io/posts/001-welcome.html",
			name: "welcome message",
		},
		"[[1]]": link{
			url:  "https://v.io",
			name: "v.io",
		},
		"[[2]]": link{
			url:  "https://groups.google.com/a/v.io/forum/#!forum/vanadium-discuss",
			name: "vanadium-discuss@v.io",
		},
		"[[3]]": link{
			url:  "https://docs.google.com/a/google.com/forms/d/1IYq3fkmgqToqzVp0EAg3Oxv_7mtDzn6VCyMiiTdPNDY/viewform",
			name: "here to sign up",
		},
	},
}

func main() {
	ctx := tool.NewDefaultContext()

	root, err := project.V23Root()
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

		if err := sendWelcomeEmail(mailer, email, attachment); err != nil {
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

func sendWelcomeEmail(mailer *gomail.Mailer, email string, attachment string) error {
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
