package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ==============================
// PDF 加解密标签页（独立功能）
// ==============================
func checkPDFEncrypted(filePath string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	info, err := api.PDFInfo(f, filePath, nil, false, conf)
	if err == nil {
		return info.Encrypted, nil
	}

	// PDFInfo 失败时回退到扫描原始字节查 /Encrypt 键（不依赖 error message locale）
	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return false, readErr
	}
	return bytes.Contains(data, []byte("/Encrypt")), nil
}

func createCryptTab(win fyne.Window) *container.TabItem {
	// ========== 加密部分 ==========
	encryptFileLabel := widget.NewLabel("未选择文件")
	encryptPass := widget.NewPasswordEntry()
	encryptPass.SetPlaceHolder("密码（必填）")
	encryptPassConfirm := widget.NewPasswordEntry()
	encryptPassConfirm.SetPlaceHolder("确认密码")

	encryptOutputEntry := widget.NewEntry()
	defaultEncryptName := fmt.Sprintf("encrypted-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))
	encryptOutputEntry.PlaceHolder = "输出文件路径"
	encryptOutputEntry.Text = defaultEncryptName

	selectEncryptFile := &widget.Button{
		Text:       "选择待加密PDF",
		Importance: widget.HighImportance,
		OnTapped: func() {
		fd := dialog.NewFileOpen(func(f fyne.URIReadCloser, err error) {
			if f == nil || err != nil {
				return
			}
			path := f.URI().Path()
			encryptFileLabel.SetText(path)

			encrypted, err := checkPDFEncrypted(path)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if encrypted {
				dialog.ShowInformation("提示", "该PDF文件已经加密，无法再次加密！", win)
			}
		}, win)
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
		},
	}

	selectEncryptOutput := &widget.Button{
		Text:       "输出",
		Importance: widget.HighImportance,
		OnTapped: func() {
			saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil || writer == nil {
					return
				}
				path := writer.URI().Path()
				if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
					path += ".pdf"
				}
				encryptOutputEntry.SetText(path)
				writer.Close()
				os.Remove(path)
			}, win)
			saveDlg.Resize(fyne.NewSize(800, 600))
			saveDlg.Show()
		},
	}

	doEncrypt := &widget.Button{
		Text:       "加密 PDF",
		Importance: widget.HighImportance,
		OnTapped: func() {
			filePath := encryptFileLabel.Text
			password := encryptPass.Text
			confirm := encryptPassConfirm.Text

			if filePath == "未选择文件" {
				dialog.ShowInformation("提示", "请选择文件", win)
				return
			}
			if password == "" {
				dialog.ShowInformation("提示", "请填写密码", win)
				return
			}
			if password != confirm {
				dialog.ShowInformation("提示", "两次密码不一致！", win)
				return
			}

			outPath := encryptOutputEntry.Text
			if outPath == "" {
				outPath = filePath + ".encrypted.pdf"
			}

			conf := model.NewAESConfiguration(password, password, 256)

			if err := api.EncryptFile(filePath, outPath, conf); err != nil {
				dialog.ShowError(err, win)
				return
			}

			dialog.ShowInformation("成功", "PDF加密完成！", win)
		},
	}

	encryptOutputRow := container.NewBorder(nil, nil, selectEncryptOutput, doEncrypt, encryptOutputEntry)

	encryptBox := container.NewPadded(
		container.NewVBox(
			widget.NewLabelWithStyle("🔒加 密：", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(
				selectEncryptFile,
				encryptFileLabel,
			),
			encryptPass,
			encryptPassConfirm,
			encryptOutputRow,
		),
	)

	// ========== 解密部分 ==========
	decryptFileLabel := widget.NewLabel("未选择文件")
	decryptPass := widget.NewPasswordEntry()
	decryptPass.SetPlaceHolder("打开密码 / 权限密码")

	decryptOutputEntry := widget.NewEntry()
	defaultDecryptName := fmt.Sprintf("decrypted-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))
	decryptOutputEntry.PlaceHolder = "输出文件路径"
	decryptOutputEntry.Text = defaultDecryptName

	selectDecryptFile := &widget.Button{
		Text:       "选择待解密PDF",
		Importance: widget.HighImportance,
		OnTapped: func() {
		fd := dialog.NewFileOpen(func(f fyne.URIReadCloser, err error) {
			if f == nil || err != nil {
				return
			}
			path := f.URI().Path()
			decryptFileLabel.SetText(path)

			encrypted, err := checkPDFEncrypted(path)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if !encrypted {
				dialog.ShowInformation("提示", "该PDF文件未加密，无需解密！", win)
			}
		}, win)
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
		},
	}

	selectDecryptOutput := &widget.Button{
		Text:       "输出",
		Importance: widget.HighImportance,
		OnTapped: func() {
			saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil || writer == nil {
					return
				}
				path := writer.URI().Path()
				if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
					path += ".pdf"
				}
				decryptOutputEntry.SetText(path)
				writer.Close()
				os.Remove(path)
			}, win)
			saveDlg.Resize(fyne.NewSize(800, 600))
			saveDlg.Show()
		},
	}

	doDecrypt := &widget.Button{
		Text:       "解密 PDF（移除密码）",
		Importance: widget.HighImportance,
		OnTapped: func() {
			filePath := decryptFileLabel.Text
			pwd := decryptPass.Text

			if filePath == "未选择文件" {
				dialog.ShowInformation("提示", "请选择文件", win)
				return
			}

			outPath := decryptOutputEntry.Text
			if outPath == "" {
				outPath = filePath + ".decrypted.pdf"
			}

			var conf *model.Configuration
			if pwd != "" {
				conf = model.NewDefaultConfiguration()
				conf.UserPW = pwd
				conf.OwnerPW = pwd
			}

			if err := api.DecryptFile(filePath, outPath, conf); err != nil {
				dialog.ShowError(err, win)
				return
			}

			dialog.ShowInformation("成功", "密码已全部移除！", win)
		},
	}

	decryptOutputRow := container.NewBorder(nil, nil, selectDecryptOutput, doDecrypt, decryptOutputEntry)

	decryptBox := container.NewPadded(
		container.NewVBox(
			widget.NewLabelWithStyle("🔓解 密：", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(
				selectDecryptFile,
				decryptFileLabel,
			),
			decryptPass,
			decryptOutputRow,
		),
	)

	// 布局：上下分栏
	content := container.NewVBox(encryptBox, widget.NewSeparator(), decryptBox)
	return container.NewTabItem("加密", container.NewVScroll(content))
}
