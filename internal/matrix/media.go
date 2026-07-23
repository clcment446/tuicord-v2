package matrix

import (
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// DownloadURL builds the authenticated client-server download URL for an mxc
// URI (MSC3916). The returned URL requires an Authorization: Bearer header,
// which the media authorizer adds.
func DownloadURL(baseURL string, uri id.ContentURI) string {
	if uri.IsEmpty() {
		return ""
	}
	return strings.TrimSuffix(baseURL, "/") +
		"/_matrix/client/v1/media/download/" + uri.Homeserver + "/" + uri.FileID
}

// ThumbnailURL builds an authenticated scaled-thumbnail URL for an mxc URI.
func ThumbnailURL(baseURL string, uri id.ContentURI, width, height int) string {
	if uri.IsEmpty() {
		return ""
	}
	return strings.TrimSuffix(baseURL, "/") +
		"/_matrix/client/v1/media/thumbnail/" + uri.Homeserver + "/" + uri.FileID +
		"?method=scale&width=" + itoa(width) + "&height=" + itoa(height)
}

// ParseMXC parses an mxc:// URI string.
func ParseMXC(mxc id.ContentURIString) (id.ContentURI, bool) {
	uri, err := mxc.Parse()
	if err != nil || uri.IsEmpty() {
		return id.ContentURI{}, false
	}
	return uri, true
}

// DecryptAttachment decrypts an encrypted attachment body using the keys carried
// in an m.room.message file field.
func DecryptAttachment(file *event.EncryptedFileInfo, ciphertext []byte) ([]byte, error) {
	return file.Decrypt(ciphertext)
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
