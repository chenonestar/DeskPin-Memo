// Package crypto 实现 6.3 的可选字段级加密：AES-256-GCM 字段加密、
// DEK 由主密码（Argon2id）/ 恢复密钥 / DPAPI 分别包裹。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	prefix       = "v1:"
	keyLen       = 32
	argonMemKiB  = 64 * 1024 // 64 MB
	argonTime    = 3
	argonThreads = 4
	saltLen      = 16
	recoveryLen  = 24
	// 恢复密钥字符集：去掉易混淆字符
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// ErrBadPassword 表示密码 / 恢复密钥错误（GCM 认证失败）。
var ErrBadPassword = errors.New("密码或恢复密钥不正确")

// Cipher 用 DEK 加解密字段，实现 store.Codec。
type Cipher struct{ aead cipher.AEAD }

// NewCipher 由 256 位 DEK 创建字段加密器。
func NewCipher(dek []byte) (*Cipher, error) {
	if len(dek) != keyLen {
		return nil, errors.New("crypto: DEK 必须为 32 字节")
	}
	b, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b) // 96 位 nonce
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: g}, nil
}

// Enc 输出 v1:nonce:ciphertext（Base64）。空串也加密，避免泄露长度为 0 的事实以外的信息。
func (c *Cipher) Enc(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := c.aead.Seal(nil, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(ct), nil
}

// Dec 解密；不带 v1: 前缀的视为明文原样返回（兼容开关加密的中间状态）。
func (c *Cipher) Dec(s string) (string, error) {
	if !IsCipher(s) {
		return s, nil
	}
	parts := strings.SplitN(s[len(prefix):], ":", 2)
	if len(parts) != 2 {
		return "", errors.New("crypto: 密文格式错误")
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	ct, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	if len(nonce) != c.aead.NonceSize() {
		return "", errors.New("crypto: nonce 长度错误")
	}
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// IsCipher 判断字符串是否为本方案密文。
func IsCipher(s string) bool { return strings.HasPrefix(s, prefix) }

// NewDEK 生成随机 256 位数据密钥。
func NewDEK() ([]byte, error) {
	k := make([]byte, keyLen)
	_, err := rand.Read(k)
	return k, err
}

// NewRecoveryKey 生成 24 位恢复密钥（每 4 位一组展示由调用方处理）。
func NewRecoveryKey() (string, error) {
	b := make([]byte, recoveryLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, recoveryLen)
	for i, v := range b {
		out[i] = recoveryAlphabet[int(v)%len(recoveryAlphabet)]
	}
	return string(out), nil
}

// NormalizeRecoveryKey 去掉空格 / 连字符并转大写。
func NormalizeRecoveryKey(s string) string {
	s = strings.ToUpper(s)
	s = strings.NewReplacer(" ", "", "-", "").Replace(s)
	return s
}

// Wrapped 是被包裹的 DEK：salt + AES-GCM(nonce||ct) 的 Base64。
type Wrapped struct {
	Salt string `json:"salt"`
	Blob string `json:"blob"`
}

func deriveKey(secret string, salt []byte) []byte {
	return argon2.IDKey([]byte(secret), salt, argonTime, argonMemKiB, argonThreads, keyLen)
}

// WrapDEK 用密码（或恢复密钥）经 Argon2id 派生密钥后包裹 DEK。
func WrapDEK(dek []byte, secret string) (Wrapped, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return Wrapped{}, err
	}
	c, err := NewCipher(deriveKey(secret, salt))
	if err != nil {
		return Wrapped{}, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Wrapped{}, err
	}
	ct := c.aead.Seal(nonce, nonce, dek, nil)
	return Wrapped{Salt: base64.StdEncoding.EncodeToString(salt), Blob: base64.StdEncoding.EncodeToString(ct)}, nil
}

// UnwrapDEK 解开被包裹的 DEK；密码错误返回 ErrBadPassword。
func UnwrapDEK(w Wrapped, secret string) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(w.Salt)
	if err != nil {
		return nil, err
	}
	blob, err := base64.StdEncoding.DecodeString(w.Blob)
	if err != nil {
		return nil, err
	}
	c, err := NewCipher(deriveKey(secret, salt))
	if err != nil {
		return nil, err
	}
	ns := c.aead.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("crypto: 包裹数据损坏")
	}
	dek, err := c.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return nil, ErrBadPassword
	}
	return dek, nil
}

// Seal 用密码加密任意数据（导出加密用），返回自描述结构。
func Seal(data []byte, password string) (Wrapped, error) { return WrapDEK(data, password) }

// Open 解开 Seal 的结果。
func Open(w Wrapped, password string) ([]byte, error) { return UnwrapDEK(w, password) }
