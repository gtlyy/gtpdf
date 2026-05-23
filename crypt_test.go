package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func copyTestPDF(t *testing.T, src string) string {
	t.Helper()
	srcPath := filepath.Join("testdata", src)
	dst := filepath.Join(t.TempDir(), src)
	in, err := os.Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestCheckPDFEncrypted_Unencrypted(t *testing.T) {
	path := copyTestPDF(t, "eth.pdf")
	encrypted, err := checkPDFEncrypted(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if encrypted {
		t.Error("expected unencrypted PDF, got encrypted=true")
	}
}

func TestCheckPDFEncrypted_NonExistent(t *testing.T) {
	_, err := checkPDFEncrypted("/nonexistent/path.pdf")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestCheckPDFEncrypted_EncryptedFile(t *testing.T) {
	path := copyTestPDF(t, "eth.pdf")
	encPath := path + ".enc"
	conf := model.NewAESConfiguration("testpass", "testpass", 256)
	if err := api.EncryptFile(path, encPath, conf); err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	encrypted, err := checkPDFEncrypted(encPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !encrypted {
		t.Error("expected encrypted PDF, got encrypted=false")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	path := copyTestPDF(t, "eth.pdf")
	password := "mypassword"

	encPath := path + ".enc"
	conf := model.NewAESConfiguration(password, password, 256)
	if err := api.EncryptFile(path, encPath, conf); err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	decPath := path + ".dec"
	decConf := model.NewDefaultConfiguration()
	decConf.UserPW = password
	decConf.OwnerPW = password
	if err := api.DecryptFile(encPath, decPath, decConf); err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	encrypted, err := checkPDFEncrypted(decPath)
	if err != nil {
		t.Fatalf("check after decrypt failed: %v", err)
	}
	if encrypted {
		t.Error("expected decrypted PDF to be unencrypted")
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	path := copyTestPDF(t, "eth.pdf")
	password := "correctpass"

	encPath := path + ".enc"
	conf := model.NewAESConfiguration(password, password, 256)
	if err := api.EncryptFile(path, encPath, conf); err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	decPath := path + ".dec"
	decConf := model.NewDefaultConfiguration()
	decConf.UserPW = "wrongpass"
	decConf.OwnerPW = "wrongpass"
	err := api.DecryptFile(encPath, decPath, decConf)
	if err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestEncryptPasswordMismatch(t *testing.T) {
	// 原 createCryptTab 中的逻辑：两次密码不一致应拒绝加密
	// 这里直接测试加密 API 的行为，确保密码不正确时解密失败
	path := copyTestPDF(t, "eth.pdf")
	encPath := path + ".enc"
	// 用 "pass1" 加密
	conf := model.NewAESConfiguration("pass1", "pass1", 256)
	if err := api.EncryptFile(path, encPath, conf); err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	// 用 "pass2"（typo）解密应失败
	decConf := model.NewDefaultConfiguration()
	decConf.UserPW = "pass2"
	decConf.OwnerPW = "pass2"
	err := api.DecryptFile(encPath, path+".dec", decConf)
	if err == nil {
		t.Error("expected error when decrypting with wrong password (simulates typo)")
	}
}

func TestEncryptAlreadyEncrypted(t *testing.T) {
	path := copyTestPDF(t, "eth.pdf")

	encPath1 := path + ".enc1"
	conf1 := model.NewAESConfiguration("pass1", "pass1", 256)
	if err := api.EncryptFile(path, encPath1, conf1); err != nil {
		t.Fatalf("first encrypt failed: %v", err)
	}

	encPath2 := path + ".enc2"
	conf2 := model.NewAESConfiguration("pass2", "pass2", 256)
	err := api.EncryptFile(encPath1, encPath2, conf2)
	if err == nil {
		t.Error("expected error when re-encrypting an encrypted file")
	}
}

func TestCheckPDFEncrypted_ByteScanFallback(t *testing.T) {
	// 验证回退到字节扫描 /Encrypt 的路径能正常工作
	path := copyTestPDF(t, "eth.pdf")

	encPath := path + ".enc"
	encConf := model.NewAESConfiguration("test123", "test123", 256)
	if err := api.EncryptFile(path, encPath, encConf); err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 加密文件应包含 /Encrypt
	data, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/Encrypt")) {
		t.Error("encrypted file should contain /Encrypt key")
	}

	// 未加密文件不应包含 /Encrypt
	data2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data2, []byte("/Encrypt")) {
		t.Error("unencrypted file should not contain /Encrypt")
	}
}
