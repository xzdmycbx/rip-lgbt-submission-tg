package auth

import (
	"errors"
	"net/url"

	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret returns a fresh TOTP secret + an otpauth URL the
// frontend can render as a QR code.
func GenerateTOTPSecret(issuer, accountName string) (secret string, otpauthURL string, err error) {
	if accountName == "" {
		return "", "", errors.New("accountName required")
	}
	if issuer == "" {
		issuer = "rip.lgbt"
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// VerifyTOTP returns nil iff code matches secret in the current 30s window
// (with the standard ±1 step skew).
func VerifyTOTP(secret, code string) error {
	if secret == "" || code == "" {
		return errors.New("missing secret or code")
	}
	ok := totp.Validate(code, secret)
	if !ok {
		return errors.New("invalid TOTP code")
	}
	return nil
}

// BuildOtpauthURL is a small helper for tests / frontend hand-off.
func BuildOtpauthURL(issuer, accountName, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	u := url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/" + url.PathEscape(issuer) + ":" + url.PathEscape(accountName),
		RawQuery: v.Encode(),
	}
	return u.String()
}
