// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command mailer sends the vanadium welcome email, including the NDA as an
attachment.  The email is sent via smtp-relay.gmail.com.

Due to legacy reasons(?), the configuration is via environment variables:
	EMAIL_USERNAME: Sender email username.
	EMAIL_PASSWORD: Sender email password.
	EMAILS:         Space-separated list of recipient email addresses.
	NDA_BUCKET:     Google-storage bucket that contains the NDA.

Usage:
   mailer [flags]

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.
*/
package main
