package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"deskpinmemo/internal/crypto"
)

// 解锁方式（FR-607）。
const (
	UnlockDPAPI    = "dpapi"    // 本机账户自动解锁（默认）
	UnlockPassword = "password" // 每次启动输入密码
)

// EncStatus 是加密状态（设置页展示）。
type EncStatus struct {
	Enabled        bool   `json:"enabled"`
	Locked         bool   `json:"locked"`
	Mode           string `json:"mode"`
	DPAPIAvailable bool   `json:"dpapiAvailable"`
}

const (
	metaPW       = "enc_wrap_password"
	metaRecovery = "enc_wrap_recovery"
	metaDPAPI    = "enc_wrap_dpapi"
	metaMode     = "enc_mode"
)

// EncryptionStatus 返回加密状态。
func (s *Service) EncryptionStatus() EncStatus {
	mode, _ := s.st.GetMeta(metaMode)
	return EncStatus{Enabled: s.st.Encrypted(), Locked: s.st.Locked(), Mode: mode, DPAPIAvailable: crypto.DPAPIAvailable}
}

func (s *Service) putWrap(key string, w crypto.Wrapped) error {
	b, _ := json.Marshal(w)
	return s.st.SetMeta(key, string(b))
}

func (s *Service) getWrap(key string) (crypto.Wrapped, bool) {
	v, ok := s.st.GetMeta(key)
	if !ok {
		return crypto.Wrapped{}, false
	}
	var w crypto.Wrapped
	return w, json.Unmarshal([]byte(v), &w) == nil
}

func (s *Service) storeDPAPI(dek []byte) error {
	blob, err := crypto.DPAPIProtect(dek)
	if err != nil {
		return err
	}
	return s.st.SetMeta(metaDPAPI, base64.StdEncoding.EncodeToString(blob))
}

// EnableEncryption 开启加密（6.3）：先备份，再在单个事务内加密全部数据。
// 返回 24 位恢复密钥，前端必须要求用户确认已保存。mode 为 dpapi 或 password。
func (s *Service) EnableEncryption(password, mode string) (recoveryKey string, err error) {
	if s.st.Encrypted() {
		return "", errors.New("加密已开启")
	}
	if len(password) < 6 {
		return "", errors.New("主密码至少 6 位")
	}
	if mode != UnlockPassword {
		mode = UnlockDPAPI
	}
	if mode == UnlockDPAPI && !crypto.DPAPIAvailable {
		mode = UnlockPassword
	}
	if _, err := s.st.BackupTo(s.BackupDir(), "pre-encrypt"); err != nil {
		return "", err
	}
	dek, err := crypto.NewDEK()
	if err != nil {
		return "", err
	}
	if recoveryKey, err = crypto.NewRecoveryKey(); err != nil {
		return "", err
	}
	pw, err := crypto.WrapDEK(dek, password)
	if err != nil {
		return "", err
	}
	rk, err := crypto.WrapDEK(dek, recoveryKey)
	if err != nil {
		return "", err
	}
	if err := s.putWrap(metaPW, pw); err != nil {
		return "", err
	}
	if err := s.putWrap(metaRecovery, rk); err != nil {
		return "", err
	}
	if mode == UnlockDPAPI {
		if err := s.storeDPAPI(dek); err != nil {
			return "", err
		}
	}
	if err := s.st.SetMeta(metaMode, mode); err != nil {
		return "", err
	}
	c, err := crypto.NewCipher(dek)
	if err != nil {
		return "", err
	}
	if err := s.st.EnableEncryption(c); err != nil {
		return "", err
	}
	s.changed()
	return recoveryKey, nil
}

func (s *Service) unlockWith(dek []byte) error {
	c, err := crypto.NewCipher(dek)
	if err != nil {
		return err
	}
	s.st.SetCodec(c)
	s.Emit("lock:changed", false)
	s.changed()
	return nil
}

// TryAutoUnlock 启动时尝试用 DPAPI 自动解锁；失败或未配置则保持锁定。
func (s *Service) TryAutoUnlock() bool {
	if !s.st.Locked() {
		return true
	}
	v, ok := s.st.GetMeta(metaDPAPI)
	if !ok {
		return false
	}
	blob, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return false
	}
	dek, err := crypto.DPAPIUnprotect(blob)
	if err != nil {
		return false
	}
	return s.unlockWith(dek) == nil
}

// Unlock 用主密码解锁。
func (s *Service) Unlock(password string) error {
	w, ok := s.getWrap(metaPW)
	if !ok {
		return errors.New("未找到密钥信息")
	}
	dek, err := crypto.UnwrapDEK(w, password)
	if err != nil {
		return err
	}
	return s.unlockWith(dek)
}

// ResetPasswordWithRecovery 用恢复密钥重设主密码（忘记主密码时）。
func (s *Service) ResetPasswordWithRecovery(recoveryKey, newPassword string) error {
	if len(newPassword) < 6 {
		return errors.New("主密码至少 6 位")
	}
	w, ok := s.getWrap(metaRecovery)
	if !ok {
		return errors.New("未找到恢复密钥信息")
	}
	dek, err := crypto.UnwrapDEK(w, crypto.NormalizeRecoveryKey(recoveryKey))
	if err != nil {
		return err
	}
	pw, err := crypto.WrapDEK(dek, newPassword)
	if err != nil {
		return err
	}
	if err := s.putWrap(metaPW, pw); err != nil {
		return err
	}
	return s.unlockWith(dek)
}

// ChangePassword 修改主密码。
func (s *Service) ChangePassword(oldPassword, newPassword string) error {
	if len(newPassword) < 6 {
		return errors.New("主密码至少 6 位")
	}
	w, ok := s.getWrap(metaPW)
	if !ok {
		return errors.New("未找到密钥信息")
	}
	dek, err := crypto.UnwrapDEK(w, oldPassword)
	if err != nil {
		return err
	}
	nw, err := crypto.WrapDEK(dek, newPassword)
	if err != nil {
		return err
	}
	return s.putWrap(metaPW, nw)
}

// SetUnlockMode 切换解锁方式：选「每次输入密码」时删除 DPAPI 包裹副本（6.3）。
func (s *Service) SetUnlockMode(mode, password string) error {
	if !s.st.Encrypted() {
		return errors.New("加密未开启")
	}
	w, ok := s.getWrap(metaPW)
	if !ok {
		return errors.New("未找到密钥信息")
	}
	dek, err := crypto.UnwrapDEK(w, password)
	if err != nil {
		return err
	}
	if mode == UnlockPassword {
		if err := s.st.SetMeta(metaDPAPI, ""); err != nil {
			return err
		}
	} else {
		if !crypto.DPAPIAvailable {
			return errors.New("当前系统不支持 DPAPI")
		}
		if err := s.storeDPAPI(dek); err != nil {
			return err
		}
	}
	return s.st.SetMeta(metaMode, mode)
}

// DisableEncryption 关闭加密：验证主密码，先备份，再批量解密。
func (s *Service) DisableEncryption(password string) error {
	w, ok := s.getWrap(metaPW)
	if !ok {
		return errors.New("未找到密钥信息")
	}
	dek, err := crypto.UnwrapDEK(w, password)
	if err != nil {
		return err
	}
	if s.st.Locked() {
		if err := s.unlockWith(dek); err != nil {
			return err
		}
	}
	if _, err := s.st.BackupTo(s.BackupDir(), "pre-decrypt"); err != nil {
		return err
	}
	if err := s.st.DisableEncryption(); err != nil {
		return err
	}
	for _, k := range []string{metaPW, metaRecovery, metaDPAPI, metaMode} {
		_ = s.st.SetMeta(k, "")
	}
	s.changed()
	return nil
}

// DeletePlaintextBackups 开启/关闭加密后，删除此前的明文备份（提示用户后调用）。
func (s *Service) DeletePlaintextBackups() (int, error) {
	files, _ := filepath.Glob(filepath.Join(s.BackupDir(), "*.db"))
	n := 0
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
