package secretbox

// https://godoc.org/golang.org/x/crypto/scrypt
// https://godoc.org/github.com/awnumar/memguard
// https://spacetime.dev/encrypting-secrets-in-memory

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/awnumar/memguard"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func init() {

	memguard.CatchInterrupt()
}

type Secretbox struct {
	enclave *memguard.Enclave
	options *SecretboxOptions
}

type SecretboxOptions struct {
	Salt   string
	Suffix string
	Debug  bool
}

func NewSecretboxOptions() *SecretboxOptions {

	opts := SecretboxOptions{
		Salt:   "",
		Suffix: "enc",
		Debug:  false,
	}

	return &opts
}

func NewSecretbox(pswd string, opts *SecretboxOptions) (*Secretbox, error) {

	r := strings.NewReader(pswd)
	return NewSecretboxWithReader(r, opts)
}

func NewSecretboxWithReader(fh io.Reader, opts *SecretboxOptions) (*Secretbox, error) {

	// PLEASE TRIPLE-CHECK opts.Salt HERE...

	N := 32768
	r := 8
	p := 1

	body, err := ioutil.ReadAll(fh)

	if err != nil {
		return nil, err
	}

	key, err := scrypt.Key(body, []byte(opts.Salt), N, r, p, 32)

	if err != nil {
		return nil, err
	}

	enclave := memguard.NewEnclave(key)
	return NewSecretboxWithEnclave(enclave, opts)
}

func NewSecretboxWithEnclave(enclave *memguard.Enclave, opts *SecretboxOptions) (*Secretbox, error) {

	sb := Secretbox{
		enclave: enclave,
		options: opts,
	}

	return &sb, nil
}

func (sb Secretbox) Lock(body []byte) (string, error) {

	var nonce [24]byte

	_, err := io.ReadFull(rand.Reader, nonce[:])

	if err != nil {
		return "", err
	}

	key, err := sb.enclave.Open()

	if err != nil {
		return "", err
	}

	defer key.Destroy()

	enc := secretbox.Seal(nonce[:], body, &nonce, key.ByteArray32())
	enc_hex := base64.StdEncoding.EncodeToString(enc)

	return enc_hex, nil
}

func (sb Secretbox) LockFile(abs_path string) (string, error) {

	root := filepath.Dir(abs_path)
	fname := filepath.Base(abs_path)

	body, err := ReadFile(abs_path)

	if err != nil {
		return "", err
	}

	enc_hex, err := sb.Lock(body)

	if err != nil {
		return "", err
	}

	enc_fname := fmt.Sprintf("%s%s", fname, sb.options.Suffix)
	enc_path := filepath.Join(root, enc_fname)

	if sb.options.Debug {
		log.Printf("debugging is enabled so don't actually write %s\n", enc_path)
		return enc_path, nil
	}

	return WriteFile([]byte(enc_hex), enc_path)
}

func (sb Secretbox) Unlock(body_hex []byte) ([]byte, error) {

	body_str, err := base64.StdEncoding.DecodeString(string(body_hex))

	if err != nil {
		return nil, err
	}

	body := []byte(body_str)

	var nonce [24]byte
	copy(nonce[:], body[:24])

	key, err := sb.enclave.Open()

	if err != nil {
		return nil, err
	}

	defer key.Destroy()

	out, ok := secretbox.Open(nil, body[24:], &nonce, key.ByteArray32())

	if !ok {
		return nil, errors.New("Unable to open secretbox")
	}

	return out, nil
}

func (sb Secretbox) UnlockFile(abs_path string) (string, error) {

	root := filepath.Dir(abs_path)
	fname := filepath.Base(abs_path)
	ext := filepath.Ext(abs_path)

	if ext != sb.options.Suffix {
		return "", errors.New("Unexpected suffix")
	}

	body_hex, err := ReadFile(abs_path)

	if err != nil {
		return "", err
	}

	out, err := sb.Unlock(body_hex)

	if err != nil {
		return "", err
	}

	out_fname := strings.TrimRight(fname, ext)
	out_path := filepath.Join(root, out_fname)

	if sb.options.Debug {
		log.Printf("debugging is enabled so don't actually write %s\n", out_path)
		return out_path, nil
	}

	return WriteFile(out, out_path)
}

func ReadFile(path string) ([]byte, error) {

	fh, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(fh)
}

func WriteFile(body []byte, path string) (string, error) {

	fh, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)

	if err != nil {
		return "", err
	}

	_, err = fh.Write(body)

	if err != nil {
		return "", err
	}

	err = fh.Close()

	if err != nil {
		return "", err
	}

	return path, nil
}
