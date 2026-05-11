package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Version = 19
)

type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2Params = Argon2Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

func HashPassword(password string) (string, error) {
	return hashPassword(password, DefaultArgon2Params)
}

func VerifyPassword(password string, encodedHash string) (bool, error) {
	params, salt, expected, err := parsePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	if subtle.ConstantTimeCompare(actual, expected) == 1 {
		return true, nil
	}
	return false, nil
}

func ValidatePasswordHash(encodedHash string) error {
	_, _, _, err := parsePasswordHash(encodedHash)
	return err
}

func hashPassword(password string, params Argon2Params) (string, error) {
	if params.Memory == 0 || params.Iterations == 0 || params.Parallelism == 0 || params.SaltLength == 0 || params.KeyLength == 0 {
		return "", errors.New("invalid argon2id parameters")
	}
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	return fmt.Sprintf("argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func parsePasswordHash(encodedHash string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 {
		return Argon2Params{}, nil, nil, errors.New("malformed password hash")
	}
	if parts[0] != "argon2id" {
		return Argon2Params{}, nil, nil, errors.New("unsupported password hash algorithm")
	}
	if parts[1] != fmt.Sprintf("v=%d", argon2Version) {
		return Argon2Params{}, nil, nil, errors.New("unsupported argon2id version")
	}

	params, err := parseArgon2Params(parts[2])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) == 0 {
		return Argon2Params{}, nil, nil, errors.New("malformed password hash salt")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hash) == 0 {
		return Argon2Params{}, nil, nil, errors.New("malformed password hash value")
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(hash))
	return params, salt, hash, nil
}

func parseArgon2Params(value string) (Argon2Params, error) {
	var params Argon2Params
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		key, raw, ok := strings.Cut(part, "=")
		if !ok {
			return Argon2Params{}, errors.New("malformed argon2id parameters")
		}
		parsed, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || parsed == 0 {
			return Argon2Params{}, errors.New("malformed argon2id parameters")
		}
		seen[key] = true
		switch key {
		case "m":
			params.Memory = uint32(parsed)
		case "t":
			params.Iterations = uint32(parsed)
		case "p":
			if parsed > 255 {
				return Argon2Params{}, errors.New("malformed argon2id parameters")
			}
			params.Parallelism = uint8(parsed)
		default:
			return Argon2Params{}, errors.New("unknown argon2id parameter")
		}
	}
	if !seen["m"] || !seen["t"] || !seen["p"] {
		return Argon2Params{}, errors.New("missing argon2id parameter")
	}
	return params, nil
}
