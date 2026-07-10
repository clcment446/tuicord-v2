package ui

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"awesomeProject/internal/discord"

	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
)

// remoteAuthGatewayURL is Discord's QR-login (remote auth) websocket endpoint.
const remoteAuthGatewayURL = "wss://remote-auth-gateway.discord.gg/?v=2"

// browserUA is a desktop browser User-Agent; the remote-auth gateway rejects
// non-browser clients.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"

// remoteAuth drives Discord's QR remote-auth protocol to obtain a user token.
//
// Flow (see Discord's undocumented remote-auth gateway): connect → hello →
// generate an RSA keypair → init(public key) → nonce_proof (prove key ownership)
// → pending_remote_init (fingerprint → QR) → user scans → pending_ticket
// (username preview) → pending_login (ticket) → exchange ticket for the
// encrypted token → decrypt with the private key.
type remoteAuth struct {
	panel *QRPanel

	conn     *websocket.Conn
	writeMu  sync.Mutex
	privKey  *rsa.PrivateKey
	fpr      string
	interval time.Duration
}

// runRemoteAuth executes the whole flow, updating the panel as it progresses.
// Any error is surfaced as a panel status message; the user can still paste a
// token instead.
func runRemoteAuth(ctx context.Context, panel *QRPanel) {
	ra := &remoteAuth{panel: panel}
	if err := ra.run(ctx); err != nil && ctx.Err() == nil && !isRemoteAuthClosed(err) {
		panel.update(func() { panel.setStatus("QR login unavailable: " + err.Error()) })
	}
}

func (ra *remoteAuth) run(ctx context.Context) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	ra.privKey = key

	header := http.Header{}
	header.Set("Origin", "https://discord.com")
	header.Set("User-Agent", browserUA)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, remoteAuthGatewayURL, header)
	if err != nil {
		return err
	}
	ra.conn = conn
	defer conn.Close()

	// Close the connection when ctx is done so the read loop unblocks.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil || isRemoteAuthClosed(err) {
				return nil
			}
			return err
		}
		done, err := ra.dispatch(ctx, data)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

func isRemoteAuthClosed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) || websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "use of closed network connection") || strings.Contains(msg, "file already closed")
}

// dispatch handles one gateway frame. It returns done=true once a token has been
// obtained and delivered.
func (ra *remoteAuth) dispatch(ctx context.Context, data []byte) (bool, error) {
	var head struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return false, err
	}

	switch head.Op {
	case "hello":
		return false, ra.onHello(ctx, data)
	case "nonce_proof":
		return false, ra.onNonceProof(data)
	case "pending_remote_init":
		return false, ra.onPendingRemoteInit(data)
	case "pending_ticket":
		return false, ra.onPendingTicket(data)
	case "pending_login":
		token, err := ra.onPendingLogin(data)
		if err != nil {
			return false, err
		}
		ra.panel.update(func() {
			ra.panel.setStatus("Logged in!")
			ra.panel.setToken(token)
		})
		return true, nil
	case "cancel":
		ra.panel.update(func() { ra.panel.setStatus("Login canceled on your phone.") })
		return true, nil
	default:
		return false, nil
	}
}

func (ra *remoteAuth) onHello(ctx context.Context, data []byte) error {
	var payload struct {
		HeartbeatInterval int `json:"heartbeat_interval"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	ra.interval = time.Duration(payload.HeartbeatInterval) * time.Millisecond
	go ra.heartbeatLoop(ctx)
	ra.panel.update(func() { ra.panel.setStatus("Requesting code…") })
	return ra.sendInit()
}

func (ra *remoteAuth) heartbeatLoop(ctx context.Context) {
	if ra.interval <= 0 {
		return
	}
	ticker := time.NewTicker(ra.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ra.writeJSON(struct {
				Op string `json:"op"`
			}{"heartbeat"}); err != nil {
				return
			}
		}
	}
}

func (ra *remoteAuth) sendInit() error {
	spki, err := x509.MarshalPKIXPublicKey(ra.privKey.Public())
	if err != nil {
		return err
	}
	return ra.writeJSON(struct {
		Op               string `json:"op"`
		EncodedPublicKey string `json:"encoded_public_key"`
	}{"init", base64.StdEncoding.EncodeToString(spki)})
}

func (ra *remoteAuth) onNonceProof(data []byte) error {
	var payload struct {
		EncryptedNonce string `json:"encrypted_nonce"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	proof, err := nonceProof(ra.privKey, payload.EncryptedNonce)
	if err != nil {
		return err
	}
	return ra.writeJSON(struct {
		Op    string `json:"op"`
		Proof string `json:"proof"`
	}{"nonce_proof", proof})
}

// nonceProof decrypts the gateway's encrypted nonce with the private key and
// returns the proof the gateway expects: the base64url-encoded SHA-256 of the
// decrypted nonce. The gateway rejects the raw nonce, so hashing is required to
// advance past nonce_proof to the QR fingerprint.
func nonceProof(priv *rsa.PrivateKey, encryptedNonce string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encryptedNonce)
	if err != nil {
		return "", err
	}
	nonce, err := rsa.DecryptOAEP(sha256.New(), nil, priv, decoded, nil)
	if err != nil {
		return "", err
	}
	proof := sha256.Sum256(nonce)
	return base64.RawURLEncoding.EncodeToString(proof[:]), nil
}

func (ra *remoteAuth) onPendingRemoteInit(data []byte) error {
	var payload struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	ra.fpr = payload.Fingerprint

	code, err := qrcode.New("https://discord.com/ra/"+payload.Fingerprint, qrcode.Low)
	if err != nil {
		return err
	}
	code.DisableBorder = false // keep the quiet zone so cameras can lock on
	matrix := code.Bitmap()
	ra.panel.update(func() {
		ra.panel.setMatrix(matrix)
		ra.panel.setStatus("Scan with the Discord mobile app.")
	})
	return nil
}

func (ra *remoteAuth) onPendingTicket(data []byte) error {
	var payload struct {
		EncryptedUserPayload string `json:"encrypted_user_payload"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.EncryptedUserPayload)
	if err != nil {
		return err
	}
	plain, err := rsa.DecryptOAEP(sha256.New(), nil, ra.privKey, decoded, nil)
	if err != nil {
		return err
	}
	parts := strings.Split(string(plain), ":")
	name := "your account"
	if len(parts) == 4 {
		name = parts[3]
	}
	ra.panel.update(func() { ra.panel.setStatus("Check your phone to approve " + name + "…") })
	return nil
}

func (ra *remoteAuth) onPendingLogin(data []byte) (string, error) {
	var payload struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	return ra.exchangeTicket(payload.Ticket)
}

// exchangeTicket trades the login ticket for the encrypted token over REST and
// decrypts it with the private key.
func (ra *remoteAuth) exchangeTicket(ticket string) (string, error) {
	headers := http.Header{}
	headers.Set("Referer", "https://discord.com/login")
	if ra.fpr != "" {
		headers.Set("X-Fingerprint", ra.fpr)
	}

	// Use the same browser-mimicking client (X-Super-Properties, Sec-Fetch-*,
	// build number, ...) as an authenticated session. Discord's fraud
	// detection expects these on the ticket exchange too; without them it
	// tends to respond with a captcha challenge instead of the token.
	client := discord.NewUnauthenticatedClient()
	client.OnRequest = append(client.OnRequest, httputil.WithHeaders(headers))

	encrypted, err := client.ExchangeRemoteAuthTicket(ticket)
	if err != nil {
		return "", err
	}
	decoded, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	token, err := rsa.DecryptOAEP(sha256.New(), nil, ra.privKey, decoded, nil)
	if err != nil {
		return "", err
	}
	if len(token) == 0 {
		return "", errors.New("empty token")
	}
	return string(token), nil
}

// writeJSON serializes v to the websocket; a mutex serializes concurrent writes
// from the heartbeat loop and the dispatch handlers.
func (ra *remoteAuth) writeJSON(v any) error {
	ra.writeMu.Lock()
	defer ra.writeMu.Unlock()
	return ra.conn.WriteJSON(v)
}
