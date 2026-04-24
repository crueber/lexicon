package email

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Service provides email send-to-device functionality.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
	secret string
}

// NewService creates a new email Service.
func NewService(db *sql.DB, logger *slog.Logger, secret string) *Service {
	return &Service{db: db, logger: logger, secret: secret}
}

// CreateProviderParams holds parameters for creating an email provider.
type CreateProviderParams struct {
	Name        string
	Host        string
	Port        int64
	Username    string
	Password    string
	FromAddress string
	UseTLS      bool
	IsDefault   bool
}

// UpdateProviderParams holds parameters for updating an email provider.
type UpdateProviderParams struct {
	Name        string
	Host        string
	Port        int64
	Username    string
	Password    string
	FromAddress string
	UseTLS      bool
	IsDefault   bool
}

// CreateRecipientParams holds parameters for creating an email recipient.
type CreateRecipientParams struct {
	UserID       int64
	Name         *string
	EmailAddress string
}

// ListProviders returns all email providers.
func (s *Service) ListProviders(ctx context.Context) ([]EmailProvider, error) {
	q := New(s.db)
	providers, err := q.ListEmailProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	return providers, nil
}

// GetProvider returns an email provider by ID.
func (s *Service) GetProvider(ctx context.Context, id int64) (EmailProvider, error) {
	q := New(s.db)
	provider, err := q.GetEmailProviderByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmailProvider{}, errors.New("provider not found")
		}
		return EmailProvider{}, fmt.Errorf("get provider: %w", err)
	}
	return provider, nil
}

// CreateProvider creates a new email provider.
func (s *Service) CreateProvider(ctx context.Context, params CreateProviderParams) (EmailProvider, error) {
	q := New(s.db)

	// If this provider is default, clear any existing default.
	if params.IsDefault {
		if err := q.ClearEmailProviderDefault(ctx); err != nil {
			return EmailProvider{}, fmt.Errorf("clear default provider: %w", err)
		}
	}

	encryptedPassword, err := encryptPassword(params.Password, s.secret)
	if err != nil {
		return EmailProvider{}, fmt.Errorf("encrypt password: %w", err)
	}

	provider, err := q.CreateEmailProvider(ctx, CreateEmailProviderParams{
		Name:        params.Name,
		Host:        params.Host,
		Port:        params.Port,
		Username:    params.Username,
		Password:    encryptedPassword,
		FromAddress: params.FromAddress,
		UseTls:      boolToInt(params.UseTLS),
		IsDefault:   boolToInt(params.IsDefault),
	})
	if err != nil {
		return EmailProvider{}, fmt.Errorf("create provider: %w", err)
	}
	return provider, nil
}

// UpdateProvider updates an existing email provider.
func (s *Service) UpdateProvider(ctx context.Context, id int64, params UpdateProviderParams) error {
	q := New(s.db)

	// Verify the provider exists.
	if _, err := q.GetEmailProviderByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("provider not found")
		}
		return fmt.Errorf("get provider: %w", err)
	}

	// If this provider is default, clear any existing default.
	if params.IsDefault {
		if err := q.ClearEmailProviderDefault(ctx); err != nil {
			return fmt.Errorf("clear default provider: %w", err)
		}
	}

	encryptedPassword, err := encryptPassword(params.Password, s.secret)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	if err := q.UpdateEmailProvider(ctx, UpdateEmailProviderParams{
		Name:        params.Name,
		Host:        params.Host,
		Port:        params.Port,
		Username:    params.Username,
		Password:    encryptedPassword,
		FromAddress: params.FromAddress,
		UseTls:      boolToInt(params.UseTLS),
		IsDefault:   boolToInt(params.IsDefault),
		ID:          id,
	}); err != nil {
		return fmt.Errorf("update provider: %w", err)
	}
	return nil
}

// DeleteProvider deletes an email provider by ID.
func (s *Service) DeleteProvider(ctx context.Context, id int64) error {
	q := New(s.db)

	if _, err := q.GetEmailProviderByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("provider not found")
		}
		return fmt.Errorf("get provider: %w", err)
	}

	if err := q.DeleteEmailProvider(ctx, id); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

// ListRecipients returns all email recipients for a user.
func (s *Service) ListRecipients(ctx context.Context, userID int64) ([]EmailRecipient, error) {
	q := New(s.db)
	recipients, err := q.ListEmailRecipientsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	return recipients, nil
}

// CreateRecipient creates a new email recipient for a user.
func (s *Service) CreateRecipient(ctx context.Context, params CreateRecipientParams) (EmailRecipient, error) {
	q := New(s.db)
	recipient, err := q.CreateEmailRecipient(ctx, CreateEmailRecipientParams{
		UserID:       params.UserID,
		Name:         nullString(params.Name),
		EmailAddress: params.EmailAddress,
	})
	if err != nil {
		return EmailRecipient{}, fmt.Errorf("create recipient: %w", err)
	}
	return recipient, nil
}

// DeleteRecipient deletes an email recipient, verifying it belongs to the user.
func (s *Service) DeleteRecipient(ctx context.Context, userID, id int64) error {
	q := New(s.db)

	recipient, err := q.GetEmailRecipientByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("recipient not found")
		}
		return fmt.Errorf("get recipient: %w", err)
	}

	if recipient.UserID != userID {
		return errors.New("recipient not found")
	}

	if err := q.DeleteEmailRecipient(ctx, id); err != nil {
		return fmt.Errorf("delete recipient: %w", err)
	}
	return nil
}

// TestProvider sends a test email using the given provider.
func (s *Service) TestProvider(ctx context.Context, providerID int64) error {
	provider, err := s.GetProvider(ctx, providerID)
	if err != nil {
		return err
	}

	subject := "Lexicon Email Test"
	body := "This is a test email from Lexicon. Your email provider is configured correctly."

	return s.sendSMTP(provider, []string{provider.FromAddress}, subject, body, "", "")
}

// SendBook sends a book file as an email attachment to the specified recipients.
func (s *Service) SendBook(ctx context.Context, bookID int64, recipientIDs []int64, providerID int64) error {
	q := New(s.db)

	// Look up the book file.
	bookFile, err := q.GetBookFileForSend(ctx, bookID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("book file not found")
		}
		return fmt.Errorf("get book file: %w", err)
	}

	// Look up recipients.
	recipients := make([]EmailRecipient, 0, len(recipientIDs))
	for _, rid := range recipientIDs {
		r, err := q.GetEmailRecipientByID(ctx, rid)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("recipient %d not found", rid)
			}
			return fmt.Errorf("get recipient: %w", err)
		}
		recipients = append(recipients, r)
	}

	// Look up provider.
	var provider EmailProvider
	if providerID > 0 {
		p, err := q.GetEmailProviderByID(ctx, providerID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("email provider not found")
			}
			return fmt.Errorf("get email provider: %w", err)
		}
		provider = p
	} else {
		p, err := q.GetDefaultEmailProvider(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("no default email provider configured")
			}
			return fmt.Errorf("get default provider: %w", err)
		}
		provider = p
	}

	// Build recipient addresses.
	to := make([]string, len(recipients))
	for i, r := range recipients {
		to[i] = r.EmailAddress
	}

	// Build subject and body.
	title := "Book"
	if bookFile.Title.Valid && bookFile.Title.String != "" {
		title = bookFile.Title.String
	}
	subject := fmt.Sprintf("Lexicon: %s", title)
	body := fmt.Sprintf("Your book \"%s\" is attached.\n\nSent from Lexicon.", title)
	attachmentName := filepath.Base(bookFile.FilePath)

	return s.sendSMTP(provider, to, subject, body, bookFile.FilePath, attachmentName)
}

// sendSMTP sends an email via SMTP with optional attachment.
func (s *Service) sendSMTP(provider EmailProvider, to []string, subject, bodyText, attachmentPath, attachmentName string) error {
	addr := net.JoinHostPort(provider.Host, strconv.Itoa(int(provider.Port)))

	var conn net.Conn
	var err error

	if provider.UseTls == 1 && provider.Port == 465 {
		tlsConfig := &tls.Config{ServerName: provider.Host}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, provider.Host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if provider.UseTls == 1 && provider.Port != 465 {
		tlsConfig := &tls.Config{ServerName: provider.Host}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("start tls: %w", err)
		}
	}

	password, err := decryptPassword(provider.Password, s.secret)
	if err != nil {
		return fmt.Errorf("decrypt provider password: %w", err)
	}

	auth := smtp.PlainAuth("", provider.Username, password, provider.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	if err := client.Mail(provider.FromAddress); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}

	for _, t := range to {
		if err := client.Rcpt(t); err != nil {
			return fmt.Errorf("rcpt to %s: %w", t, err)
		}
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	defer wc.Close()

	if err := writeEmailMessage(wc, provider.FromAddress, to, subject, bodyText, attachmentPath, attachmentName); err != nil {
		return fmt.Errorf("write email message: %w", err)
	}

	return nil
}

// writeEmailMessage writes a MIME multipart email message to w.
func writeEmailMessage(w io.Writer, from string, to []string, subject, bodyText, attachmentPath, attachmentName string) error {
	boundary := generateBoundary()

	fmt.Fprintf(w, "From: %s\r\n", from)
	fmt.Fprintf(w, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(w, "Subject: %s\r\n", subject)
	fmt.Fprintf(w, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(w, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
	fmt.Fprintf(w, "\r\n")

	mpWriter := multipart.NewWriter(w)
	if err := mpWriter.SetBoundary(boundary); err != nil {
		return fmt.Errorf("set boundary: %w", err)
	}

	// Text part.
	textHeaders := textproto.MIMEHeader{}
	textHeaders.Set("Content-Type", "text/plain; charset=\"utf-8\"")
	textPart, err := mpWriter.CreatePart(textHeaders)
	if err != nil {
		return fmt.Errorf("create text part: %w", err)
	}
	if _, err := io.WriteString(textPart, bodyText); err != nil {
		return fmt.Errorf("write text part: %w", err)
	}

	// Attachment part.
	if attachmentPath != "" {
		fileHeaders := textproto.MIMEHeader{}
		fileHeaders.Set("Content-Type", "application/octet-stream")
		fileHeaders.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, mime.QEncoding.Encode("utf-8", strings.ReplaceAll(attachmentName, `"`, ``))))
		fileHeaders.Set("Content-Transfer-Encoding", "base64")
		filePart, err := mpWriter.CreatePart(fileHeaders)
		if err != nil {
			return fmt.Errorf("create file part: %w", err)
		}

		file, err := os.Open(attachmentPath)
		if err != nil {
			return fmt.Errorf("open attachment: %w", err)
		}
		defer file.Close()

		b64 := base64.NewEncoder(base64.StdEncoding, filePart)
		if _, err := io.Copy(b64, file); err != nil {
			return fmt.Errorf("encode attachment: %w", err)
		}
		if err := b64.Close(); err != nil {
			return fmt.Errorf("close base64 encoder: %w", err)
		}
	}

	if err := mpWriter.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	return nil
}

// generateBoundary creates a random MIME boundary string.
func generateBoundary() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails.
		return fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("----=_Part_%s", hex.EncodeToString(b))
}

// boolToInt converts a bool to an int64 (0 or 1).
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nullString converts a *string to sql.NullString.
func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func deriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func encryptPassword(plaintext, secret string) (string, error) {
	key := deriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptPassword(ciphertextB64, secret string) (string, error) {
	key := deriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
