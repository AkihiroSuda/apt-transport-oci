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
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

const (
	CodeCapabilities   = 100
	CodeLog            = 101
	CodeStatus         = 102
	CodeWarning        = 104
	CodeURIStart       = 200
	CodeURIDone        = 201
	CodeURIFailure     = 400
	CodeGeneralFailure = 401
	CodeURIAcquire     = 600
	CodeConfiguration  = 601
)

var descriptions = map[int]string{
	CodeCapabilities:   "Capabilities",
	CodeLog:            "Log",
	CodeStatus:         "Status",
	CodeWarning:        "Warning",
	CodeURIStart:       "URI Start",
	CodeURIDone:        "URI Done",
	CodeURIFailure:     "URI Failure",
	CodeGeneralFailure: "General Failure",
	CodeURIAcquire:     "URI Acquire",
	CodeConfiguration:  "Configuration",
}

// Message represents a generic message as read from os.Stdin.
type Message struct {
	StatusCode  int
	Description string
	Fields      textproto.MIMEHeader
}

// Field represents a value field in a message.
type Field struct {
	Key   string
	Value string
}

// MessageReader implements an interface for reading messages from an input
// stream.
type MessageReader struct {
	reader *textproto.Reader
}

// NewMessageReader creates a new MessageReader instance.
//
// This function sets the underlying textproto.Reader.
func NewMessageReader(reader io.Reader) *MessageReader {
	return &MessageReader{reader: textproto.NewReader(bufio.NewReader(reader))}
}

// ReadMessage reads a full message from the input.
//
// This function reads lines until a Message is complete and then returns that.
func (r *MessageReader) ReadMessage() (*Message, error) {
	line, err := r.reader.ReadLine()
	for err == nil && line == "" { // blank lines between messages
		line, err = r.reader.ReadLine()
	}
	if err != nil {
		return nil, err
	}
	msg, err := parseHeader(line)
	if err != nil {
		return nil, err
	}
	// A message cut off by EOF is dropped.
	if msg.Fields, err = r.reader.ReadMIMEHeader(); err != nil {
		return nil, err
	}
	return msg, nil
}

func parseHeader(line string) (*Message, error) {
	statusCodeStr, desc, _ := strings.Cut(line, " ")
	statusCode, err := strconv.Atoi(statusCodeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid header %q: %w", line, err)
	}
	return &Message{StatusCode: statusCode, Description: strings.TrimSpace(desc)}, nil
}

// MessageWriter is a wrapper around an io.Writer which writes APT messages.
type MessageWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewMessageWriter creates a new MessageWriter.
func NewMessageWriter(w io.Writer) *MessageWriter {
	return &MessageWriter{w: w}
}

// WriteMessage writes a generic message with the given status code and fields.
func (mw *MessageWriter) WriteMessage(statusCode int, fields ...Field) {
	var buf strings.Builder
	fmt.Fprintf(&buf, "%d %s\n", statusCode, descriptions[statusCode])
	for _, field := range fields {
		if field.Value != "" {
			fmt.Fprintf(&buf, "%s: %s\n", field.Key, sanitize(field.Value))
		}
	}
	buf.WriteByte('\n')

	mw.mu.Lock()
	defer mw.mu.Unlock()
	io.WriteString(mw.w, buf.String())
}

// sanitize replaces control and format characters with spaces.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
}
