/*
   Copyright the cloudflare Authors.

   Original source: https://github.com/cloudflare/apt-transport-cloudflared/blob/042a4c0e142f1fbd81aaf1a14f1e7f74f59109cd/apt/message.go
   We copy the source from upstream apt-transport-cloudflared for easier packaging
   on debian system. We can delete this file and add upstream apt-transport-cloudflared
   back when:
   1, apt-transport-cloudflared is packaged in debian
   2, apt-transport-oci can use the packaged apt-transport-cloudflared in debian

   Licensed under the BSD 3-clause License;
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       https://opensource.org/license/bsd-3-clause

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package apt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CapFlags represents a set of Apt Capabilities.
type CapFlags int

const (
	// CapSingleInstance indicates to Apt that the method may only have one
	// instance running at any given time.
	CapSingleInstance CapFlags = 0x01

	// CapPipeline indicates to Apt that it may send multiple requests to the
	// method without waiting for each one to finish.
	CapPipeline CapFlags = 0x02

	// CapSendConfig tells Apt to send a configuration message back with the
	// config values.
	CapSendConfig CapFlags = 0x04

	// CapLocalOnly indicates to Apt that the resources are fetched locally.
	CapLocalOnly CapFlags = 0x08

	// CapNeedsCleanup tells Apt not to terminate the process when done.
	CapNeedsCleanup CapFlags = 0x10

	// CapRemovable tells Apt the method is working on a removable medium such
	// as a CDROM or USB drive.
	CapRemovable CapFlags = 0x20

	// CapAuxRequests indicates to Apt that the method handles AuxRequests.
	CapAuxRequests CapFlags = 0x40
)

// Message represents a generic message as read from os.Stdin.
type Message struct {
	StatusCode  uint64
	Description string
	Fields      map[string]string
}

// Field represents a value field in a mesage.
type Field struct {
	Key   string
	Value string
}

// NewMessage creates a new message with the given fields.
func NewMessage(statusCode uint64, description string, fields ...Field) *Message {
	fieldmap := make(map[string]string)
	for _, field := range fields {
		fieldmap[field.Key] = field.Value
	}

	return &Message{
		statusCode,
		description,
		fieldmap,
	}
}

// MessageReader implements an interface for reading messages from an input
// stream.
type MessageReader struct {
	reader  *bufio.Reader
	message *Message
}

// NewMessageReader creates a new MessageReader instance.
//
// This function sets the underlying bufio.Reader and sets the state such that
// there is no currently processing message.
func NewMessageReader(reader *bufio.Reader) *MessageReader {
	return &MessageReader{
		reader,
		nil,
	}
}

// errorGroup is a helper function for creating an error that represents
// several errors.
func errorGroup(header string, errs []error) error {
	var sb strings.Builder
	sb.WriteString(header)
	for _, e := range errs {
		sb.WriteString("\n  ")
		sb.WriteString(e.Error())
	}
	return errors.New(sb.String())
}

// ReadMessage reads a full message from the input.
//
// This function calls MessageReader.ReadLine() until a Message is returned
// and then returns that.
func (r *MessageReader) ReadMessage() (*Message, error) {
	var errs []error
	var err error
	var msg *Message

	for msg == nil {
		msg, err = r.ReadLine()
		if err != nil {
			if msg != nil || err == io.EOF {
				if len(errs) > 0 {
					errs = append(errs, err)
					return msg, errorGroup("Errors while reading message:", errs)
				}
				return msg, err
			}
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return msg, errorGroup("Errors while reading message:", errs)
	}
	return msg, nil
}

// ReadLine reads a line from the input and processes it.
//
// This function will read exactly 1 line from the input Reader, and do one of
// a few things depending on state and the value of the line.
// If no Message is currently being parsed, then this method will attempt to
// read a header line and start a new Message instance.
// If there is a Message being processed, then it will attempt to parse the
// line as a Field (Name: Value). If the line is empty, then the message is
// considered done and is returned.
func (r *MessageReader) ReadLine() (*Message, error) {
	if r.message == nil {
		msg, err := r.readHeader()
		if err != nil {
			return nil, err
		}
		r.message = msg
		return nil, nil
	}

	line, err := r.reader.ReadString('\n')
	if err != nil {
		// EOF or other stream error
		return r.commitMessage(nil), err
	}

	line = strings.TrimSpace(line)
	if line == "" {
		// Blank line in input signals end of message
		return r.commitMessage(nil), nil
	}

	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		// Bad field format - continue parsing?
		// Test for header
		msg, err := ParseHeader(line)
		if err != nil {
			return nil, fmt.Errorf("invalid field format in \"%s\"", line)
		}

		return r.commitMessage(msg), errors.New("new message started without old message ending")
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	r.message.Fields[key] = value
	return nil, nil
}

// readHeader reads a header from the input.
//
// This function will attempt to read a header line from the input. If the
// line read is empty, this function returns (nil, nil). Otherwise it will
// attempt to read an unsigned integer and then a description. Both must be
// present for the method to succeed. If a header is parsed, this method will
// return it without setting the state to reflect that.
func (r *MessageReader) readHeader() (*Message, error) {
	line, err := r.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	return ParseHeader(line)
}

// ParseHeader attempts to parse a header out of the given string.
func ParseHeader(line string) (*Message, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, errors.New("not a header spec: Empty line")
	}

	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("not a header spec: \"%s\"", line)
	}

	codeStr := strings.TrimSpace(parts[0])
	code, err := strconv.ParseUint(codeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse Status Code: %v", err)
	}

	desc := strings.TrimSpace(parts[1])
	if desc == "" {
		return nil, fmt.Errorf("empty description header for \"%s\"", line)
	}

	msg := &Message{
		StatusCode:  code,
		Description: desc,
		Fields:      make(map[string]string),
	}

	return msg, nil
}

// commitMessage returns the current message and sets the message pointer to
// nil.
func (r *MessageReader) commitMessage(newmsg *Message) *Message {
	msg := r.message
	r.message = newmsg
	return msg
}

// MessageWriter is a wrapper around an io.Writer which writes APT messages.
type MessageWriter struct {
	w io.Writer
}

// NewMessageWriter creates a new MessageWriter.
func NewMessageWriter(w io.Writer) *MessageWriter {
	return &MessageWriter{w}
}

// WriteMessage writes a generic Message object as created by NewMessage.
//
// This method is less efficient than the dedicated message functions, as it
// has to format every part of the message.
func (mw *MessageWriter) WriteMessage(msg *Message) {
	fmt.Fprintf(mw.w, "%d %s\n", msg.StatusCode, msg.Description)
	for k, v := range msg.Fields {
		if k != "" && v != "" {
			fmt.Fprintf(mw.w, "%s: %s\n", k, v)
		}
	}
	mw.w.Write([]byte("\n"))
}

// Capabilities writes a '100 Capabilities' message.
//
// Version must be non-empty. caps may be 0 for no capabilities, though
// it probably should at least be CapSendConfig (or CapDefault)
func (mw *MessageWriter) Capabilities(version string, caps CapFlags) {
	fmt.Fprintf(mw.w, "100 Capabilities\nVersion: %s\n", version)
	if caps&CapSendConfig != 0 {
		mw.w.Write([]byte("Send-Config: true\n"))
	}
	if caps&CapPipeline != 0 {
		mw.w.Write([]byte("Pipeline: true\n"))
	}
	if caps&CapSingleInstance != 0 {
		mw.w.Write([]byte("Single-Instance: true\n"))
	}
	if caps&CapLocalOnly != 0 {
		mw.w.Write([]byte("Local-Only: true\n"))
	}
	if caps&CapNeedsCleanup != 0 {
		mw.w.Write([]byte("Needs-Cleanup: true\n"))
	}
	if caps&CapRemovable != 0 {
		mw.w.Write([]byte("Removable: true\n"))
	}
	if caps&CapAuxRequests != 0 {
		mw.w.Write([]byte("AuxRequests: true\n"))
	}
	mw.w.Write([]byte("\n"))
}

// Log writes a '101 Log' message.
func (mw *MessageWriter) Log(msg string) {
	fmt.Fprintf(mw.w, "101 Log\nMessage: %s\n\n", msg)
}

// Logf writes a '101 Log' message and formats the arguments into it.
func (mw *MessageWriter) Logf(fmtspec string, args ...interface{}) {
	mw.Log(fmt.Sprintf(fmtspec, args...))
}

// Status writes a '102 status' message.
func (mw *MessageWriter) Status(msg string) {
	fmt.Fprintf(mw.w, "102 Status\nMessage: %s\n\n", msg)
}

// Statusf writes a '102 status' message and formats the arguments into if.
func (mw *MessageWriter) Statusf(fmtspec string, args ...interface{}) {
	mw.Status(fmt.Sprintf(fmtspec, args...))
}

// Redirect writes a '103 Redirect' message
func (mw *MessageWriter) Redirect(uri, newURI, altURIs string, usedMirror bool) {
	fmt.Fprintf(mw.w, "103 Redirect\nURI: %s\nNew-URI: %s\n", uri, newURI)
	if usedMirror {
		mw.w.Write([]byte("UsedMirror: true\n"))
	}
	if altURIs != "" {
		fmt.Fprintf(mw.w, "Alt-URIs: %s\n", altURIs)
	}
	mw.w.Write([]byte("\n"))
}

// Warning writes a '104 Warning' message.
func (mw *MessageWriter) Warning(msg string) {
	fmt.Fprintf(mw.w, "104 Warning\nMessage: %s\n\n", msg)
}

// Warningf writes a '104 Warning' message and formats the arguments into it.
func (mw *MessageWriter) Warningf(fmtspec string, args ...interface{}) {
	mw.Warning(fmt.Sprintf(fmtspec, args...))
}

// StartURI writes a '200 URI Start' message.
func (mw *MessageWriter) StartURI(uri, resumePoint string, size int64, usedMirror bool) {
	fmt.Fprintf(mw.w, "200 URI Start\nURI: %s\n", uri)
	if resumePoint != "" {
		fmt.Fprintf(mw.w, "Resume-Point: %s\n", resumePoint)
	}
	if size > 0 {
		fmt.Fprintf(mw.w, "Size: %d\n", size)
	}
	if usedMirror {
		mw.w.Write([]byte("UsedMirror: true\n"))
	}
	mw.w.Write([]byte("\n"))
}

// FinishURI writes a '201 URI Done' message.
func (mw *MessageWriter) FinishURI(uri, filename, resumePoint, altIMSHit string,
	imsHit, usedMirror bool, extra ...Field) {

	fmt.Fprintf(mw.w, "201 URI Done\nURI: %s\nFilename: %s\n", uri, filename)
	if resumePoint != "" {
		fmt.Fprintf(mw.w, "Resume-Point: %s\n", resumePoint)
	}
	if imsHit {
		mw.w.Write([]byte("IMS-Hit: true\n"))
	}
	if altIMSHit != "" {
		fmt.Fprintf(mw.w, "Alt-IMS-Hit: %s\n", altIMSHit)
	}
	if usedMirror {
		mw.w.Write([]byte("UsedMirror: true\n"))
	}

	// TODO: Make this better...
	for _, s := range extra {
		fmt.Fprintf(mw.w, "%s: %s\n", s.Key, s.Value)
	}

	mw.w.Write([]byte("\n"))
}

// AuxRequest writes a '351 Aux Request' message.
func (mw *MessageWriter) AuxRequest(uri, auxURI, descShort, descLong string, maximumSize uint64, usedMirror bool) {
	fmt.Fprintf(mw.w, "351 Aux Request\nURI: %s\n", uri)
	if auxURI != "" {
		fmt.Fprintf(mw.w, "Aux-URI: %s\n", auxURI)
	}
	if maximumSize > 0 {
		fmt.Fprintf(mw.w, "MaximumSize: %d\n", maximumSize)
	}
	if descShort != "" {
		fmt.Fprintf(mw.w, "Aux-ShortDesc: %s\n", descShort)
	}
	if descLong != "" {
		fmt.Fprintf(mw.w, "Aux-Description: %s\n", descLong)
	}
	if usedMirror {
		mw.w.Write([]byte("UsedMirror: true\n"))
	}
	mw.w.Write([]byte("\n"))
}

// FailedURI writes a '400 URI Failure' message.
//
// The message parameter should be "" unless the intent is to send a malformed
// URI Failure message
// failReason is only used if transientError is false
func (mw *MessageWriter) FailedURI(uri, message, failReason string, transientError, usedMirror bool) {
	mw.w.Write([]byte("400 URI Failure\n"))
	if uri == "" {
		fmt.Fprintf(mw.w, "Message: %s\n\n", message)
		return
	}
	fmt.Fprintf(mw.w, "URI: %s\n", uri)

	if transientError {
		mw.w.Write([]byte("Transient-Failure: true\n"))
	} else {
		fmt.Fprintf(mw.w, "FailReason: %s\n", failReason)
	}
	if usedMirror {
		mw.w.Write([]byte("UsedMirror: true\n"))
	}
	mw.w.Write([]byte("\n"))
}

// GeneralFailure writes a '401 General Failure' message.
func (mw *MessageWriter) GeneralFailure(msg string) {
	fmt.Fprintf(mw.w, "401 General Failure\nMessage: %s\n\n", msg)
}

// GeneralFailuref writes a '401 General Failure' message and formats the
// arguments into it.
func (mw *MessageWriter) GeneralFailuref(fmtspec string, args ...interface{}) {
	mw.GeneralFailure(fmt.Sprintf(fmtspec, args...))
}

// MediaChange writes a '403 Media Change' message.
func (mw *MessageWriter) MediaChange(media, drive string) {
	fmt.Fprintf(mw.w, "403 Media Change\nMedia: %s\nDrive: %s\n\n", media, drive)
}
