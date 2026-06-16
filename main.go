package main

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"gtpdf/pdfium_plus"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

//go:embed wqy-microhei.ttc SourceHanSansSC-Regular.otf
var fontFS embed.FS

type PDFFile struct {
	path      string
	pathLabel *widget.Label
	pageEntry *widget.Entry
}

var pdfFiles []PDFFile
var fileContainer *fyne.Container
var mergeWin fyne.Window

var splitInputPath string
var splitInputLabel *widget.Label
var splitModeRadio *widget.RadioGroup
var splitPageCountEntry *widget.Entry
var splitPageRangeEntry *widget.Entry
var splitOutputDir string
var splitOutputDirLabel *widget.Label
var splitPreviewLabel *widget.Label

var reorderInputPath string
var reorderInputLabel *widget.Label
var reorderPageCount int
var reorderPagesContainer *fyne.Container
var reorderPages []int
var reorderSelected int = -1
var reorderSecondSelected int = -1
var reorderPageWidgets []*widget.Button
var reorderWin fyne.Window

var pageNumInputPath string
var pageNumInputLabel *widget.Label
var pageNumPosition string = "br"
var pageNumFontSize int = 8
var pageNumColor string = "#000000"
var pageNumOddEvenCheck *widget.Check
var pageNumPageCount int
var pageNumMargin int = 16
var pageNumFormatSelect *widget.Select

var rotateInputPath string
var rotateInputLabel *widget.Label
var rotateAngle int = 90
var rotateRangeEntry *widget.Entry
var rotateRangeRadio *widget.RadioGroup
var rotatePageCount int

func main() {
	defer pdfium_plus.Cleanup()
	initRecent()

	myApp := app.NewWithID("com.gtpdf.app")
	myApp.SetIcon(resourceIconPng)
	myApp.Settings().SetTheme(newSubtleTheme())

	mainWindow := myApp.NewWindow("GtPDF")

	filePath := ""
	if len(os.Args) > 1 {
		abs, err := filepath.Abs(os.Args[1])
		if err == nil {
			filePath = abs
		}
	}

	showMainWindow(mainWindow, filePath)

	mainWindow.Resize(fyne.NewSize(1122, 730))
	mainWindow.ShowAndRun()
}

func showMainWindow(win fyne.Window, filePath ...string) {
	readerTabPlus := createReaderTabPlus(win, filePath...)
	mergeTab := createMergeTab(win)
	splitTab := createSplitTab(win)
	reorderTab := createReorderTab(win)
	pageNumTab := createPageNumTab(win)
	rotateTab := createRotateTab(win)
	cryptTab := createCryptTab(win)
	img2pdfTab := createImg2PDFTab(win)
	aboutTab := createAboutTab(win)

	tabs := container.NewAppTabs(
		readerTabPlus,
		mergeTab,
		splitTab,
		reorderTab,
		pageNumTab,
		rotateTab,
		cryptTab,
		img2pdfTab,
		aboutTab,
	)
	tabs.SetTabLocation(container.TabLocationTop)

	win.SetContent(tabs)
}

func createMergeTab(win fyne.Window) *container.TabItem {
	mergeWin = win
	fileContainer = container.NewVBox()

	rebuildFileList()

	content := container.NewVBox(
		widget.NewLabel("选择要合并的 PDF 文件"),
		widget.NewSeparator(),
		fileContainer,
	)

	addFileBtn := widget.NewButton("+ 添加文件", func() {
		pdfFiles = append(pdfFiles, PDFFile{
			pathLabel: widget.NewLabel("未选择文件"),
			pageEntry: widget.NewEntry(),
		})
		rebuildFileList()
	})

	outputEntry := widget.NewEntry()
	defaultName := fmt.Sprintf("out-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))
	outputEntry.PlaceHolder = "输出文件路径"
	outputEntry.Text = defaultName

	mergeBtn := &widget.Button{
		Text: "开始合并",
		OnTapped: func() {
			err := mergePDFs(outputEntry.Text, win)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			dialog.ShowInformation("成功", "PDF 合并完成！", win)
		},
		Importance: widget.HighImportance,
	}

	selectOutputBtn := &widget.Button{
		Text: "输出",
		OnTapped: func() {
			saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil || writer == nil {
					return
				}
				path := writer.URI().Path()
				if !strings.HasSuffix(path, ".pdf") {
					path += ".pdf"
				}
				outputEntry.SetText(path)
				writer.Close()
				os.Remove(path)
			}, win)
			saveDlg.Resize(fyne.NewSize(800, 600))
			saveDlg.Show()
		},
		Importance: widget.HighImportance,
	}

	outputRow := container.NewBorder(nil, nil, selectOutputBtn, mergeBtn, outputEntry)

	hintLabel := widget.NewLabel("格式示例: 1-3,5,9-11 表示第1到3页、第5页、第9到11页")

	content.Add(addFileBtn)
	content.Add(widget.NewSeparator())
	content.Add(outputRow)
	content.Add(hintLabel)

	mainContainer := container.NewVScroll(content)

	return container.NewTabItem("合并", mainContainer)
}

func createImg2PDFTab(win fyne.Window) *container.TabItem {
	var imgFiles []string
	fileList := container.NewVBox()

	var rebuildImgList func()
	rebuildImgList = func() {
		fileList.Objects = nil
		for i, f := range imgFiles {
			idx := i
			row := container.NewHBox(
				widget.NewLabel(fmt.Sprintf("%d.", idx+1)),
				widget.NewLabel(filepath.Base(f)),
			)
			removeBtn := widget.NewButton("删除", func() {
				imgFiles = append(imgFiles[:idx], imgFiles[idx+1:]...)
				rebuildImgList()
			})
			row.Add(layout.NewSpacer())
			row.Add(removeBtn)
			fileList.Add(row)
		}
	}

	addBtn := widget.NewButton("+ 添加图片", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			imgFiles = append(imgFiles, reader.URI().Path())
			reader.Close()
			rebuildImgList()
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".bmp", ".tiff", ".tif"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})

	pageSizeEntry := widget.NewSelectEntry([]string{"A4", "A3", "A5", "Letter", "Legal", "Auto"})
	pageSizeEntry.SetText("A4")
	pageSizeEntry.PlaceHolder = "页面大小"

	outputEntry := widget.NewEntry()
	outputEntry.PlaceHolder = "输出文件路径"
	outputEntry.Text = fmt.Sprintf("images-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))

	convertBtn := &widget.Button{
		Text: "开始转换",
		OnTapped: func() {
			if len(imgFiles) == 0 {
				dialog.ShowInformation("提示", "请先添加图片", win)
				return
			}
			outPath := outputEntry.Text
			if outPath == "" {
				dialog.ShowInformation("提示", "请设置输出路径", win)
				return
			}
			if !strings.HasSuffix(strings.ToLower(outPath), ".pdf") {
				outPath += ".pdf"
			}

			imp := pdfcpu.DefaultImportConfig()
			if pageSizeEntry.Text == "Auto" {
				imp.PageSize = ""
			} else {
				imp.PageSize = pageSizeEntry.Text
			}
			conf := model.NewDefaultConfiguration()

			err := api.ImportImagesFile(imgFiles, outPath, imp, conf)
			if err != nil {
				dialog.ShowError(fmt.Errorf("转换失败: %w", err), win)
				return
			}
			dialog.ShowInformation("成功", fmt.Sprintf("已生成 PDF: %s", filepath.Base(outPath)), win)
		},
		Importance: widget.HighImportance,
	}

	selectOutputBtn := &widget.Button{
		Text: "输出",
		OnTapped: func() {
			saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil || writer == nil {
					return
				}
				path := writer.URI().Path()
				if !strings.HasSuffix(path, ".pdf") {
					path += ".pdf"
				}
				outputEntry.SetText(path)
				writer.Close()
				os.Remove(path)
			}, win)
			saveDlg.Resize(fyne.NewSize(800, 600))
			saveDlg.Show()
		},
		Importance: widget.HighImportance,
	}

	outputRow := container.NewBorder(nil, nil, selectOutputBtn, convertBtn, outputEntry)

	content := container.NewVBox(
		widget.NewLabel("选择图片转换为 PDF"),
		widget.NewSeparator(),
		addBtn,
		fileList,
		widget.NewSeparator(),
		widget.NewLabel("页面大小:"),
		pageSizeEntry,
		widget.NewSeparator(),
		outputRow,
	)

	mainContainer := container.NewVScroll(content)
	return container.NewTabItem("图片", mainContainer)
}

func rebuildFileList() {
	fileContainer.Objects = nil
	for i := range pdfFiles {
		fileContainer.Add(createFileRow(i))
	}
}

func createFileRow(index int) *fyne.Container {
	selectBtn := &widget.Button{
		Text: "选择",
		OnTapped: func() {
			fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil || reader == nil {
					return
				}
				path := reader.URI().Path()
				lowerPath := strings.ToLower(path)
				if !strings.HasSuffix(lowerPath, ".pdf") {
					dialog.ShowError(fmt.Errorf("请选择PDF文件"), mergeWin)
					return
				}
				pdfFiles[index].path = path
				fileName := getFileName(pdfFiles[index].path)
				pdfFiles[index].pathLabel.SetText(fileName)

				pc, _ := safePageCountFile(pdfFiles[index].path)
				if pc > 0 {
					pdfFiles[index].pageEntry.SetText(fmt.Sprintf("1-%d", pc))
				}
			}, mergeWin)
			fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
			fd.Resize(fyne.NewSize(800, 600))
			fd.Show()
		},
		Importance: widget.HighImportance,
	}

	removeBtn := &widget.Button{
		Text: "删除",
		OnTapped: func() {
			if len(pdfFiles) <= 1 {
				dialog.ShowInformation("提示", "至少需要保留一个文件", mergeWin)
				return
			}
			pdfFiles = append(pdfFiles[:index], pdfFiles[index+1:]...)
			rebuildFileList()
		},
		Importance: widget.HighImportance,
	}

	if pdfFiles[index].path != "" {
		pdfFiles[index].pathLabel.SetText(getFileName(pdfFiles[index].path))
	}

	pageScroll := container.NewScroll(pdfFiles[index].pageEntry)
	pageScroll.SetMinSize(fyne.NewSize(120, 32))
	pdfFiles[index].pageEntry.PlaceHolder = "如: 1-3,5,9-11"

	return container.NewHBox(
		widget.NewLabel(fmt.Sprintf("文件 %d:", index+1)),
		selectBtn,
		pdfFiles[index].pathLabel,
		layout.NewSpacer(),
		widget.NewLabel(" 页码:"),
		pageScroll,
		removeBtn,
	)
}

func createSplitTab(win fyne.Window) *container.TabItem {
	content := container.NewVBox()

	splitInputLabel = widget.NewLabel("未选择文件")
	selectFileBtn := widget.NewButton("选择文件", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			path := reader.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				dialog.ShowError(fmt.Errorf("请选择PDF文件"), win)
				return
			}
			splitInputPath = path
			pc, _ := safePageCountFile(path)
			splitInputLabel.SetText(fmt.Sprintf("%s (共 %d 页)", getFileName(path), pc))
			updateSplitPreview()
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})
	selectFileBtn.Importance = widget.HighImportance

	fileRow := container.NewHBox(
		widget.NewLabel("输入文件:"),
		selectFileBtn,
		splitInputLabel,
	)

	content.Add(fileRow)
	content.Add(widget.NewSeparator())

	splitModeRadio = widget.NewRadioGroup([]string{"每 N 页拆分", "按页码拆分"}, func(value string) {
		updateSplitPreview()
	})
	splitModeRadio.Selected = "每 N 页拆分"

	modeRow := container.NewHBox(
		widget.NewLabel("拆分方式:"),
		splitModeRadio,
	)

	content.Add(modeRow)

	splitPageCountEntry = widget.NewEntry()
	splitPageCountEntry.Text = "5"
	splitPageCountEntry.PlaceHolder = "每页数"
	splitPageCountEntry.OnChanged = func(s string) {
		updateSplitPreview()
	}

	splitPageRangeEntry = widget.NewEntry()
	splitPageRangeEntry.PlaceHolder = "如: 5, 9, 12 (在第5、9、12页前拆分)"
	splitPageRangeEntry.OnChanged = func(s string) {
		updateSplitPreview()
	}

	countRow := container.NewHBox(
		widget.NewLabel("每 "),
		splitPageCountEntry,
		widget.NewLabel(" 页一个文件"),
	)
	content.Add(countRow)
	content.Add(splitPageRangeEntry)

	splitOutputDirLabel = widget.NewLabel("未选择")
	selectDirBtn := widget.NewButton("选择输出目录", func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			splitOutputDir = uri.Path()
			splitOutputDirLabel.SetText(splitOutputDir)
			updateSplitPreview()
		}, win)
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})
	selectDirBtn.Importance = widget.HighImportance

	dirRow := container.NewHBox(
		widget.NewLabel("输出目录:"),
		selectDirBtn,
		splitOutputDirLabel,
	)
	content.Add(dirRow)

	content.Add(widget.NewSeparator())

	splitPreviewLabel = widget.NewLabel("")
	content.Add(splitPreviewLabel)

	execBtn := widget.NewButton("开始拆分", func() {
		err := doSplit(win)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		dialog.ShowInformation("成功", "PDF 拆分完成！", win)
	})
	execBtn.Importance = widget.HighImportance
	content.Add(execBtn)

	hintLabel := widget.NewLabel("提示: 输入拆分点页码，如 5,9,12 表示在第5、9、12页前切开")
	content.Add(hintLabel)

	mainContainer := container.NewVScroll(content)

	return container.NewTabItem("拆分", mainContainer)
}

func updateSplitPreview() {
	if splitInputPath == "" {
		splitPreviewLabel.SetText("")
		return
	}

	pageCount, _ := safePageCountFile(splitInputPath)
	baseName := getFileName(splitInputPath)

	if splitModeRadio.Selected == "每 N 页拆分" {
		n, err := strconv.Atoi(splitPageCountEntry.Text)
		if err != nil || n <= 0 {
			splitPreviewLabel.SetText("请输入有效的页数")
			return
		}
		parts := (pageCount + n - 1) / n
		preview := generateNPageSplitPreview(baseName, n, pageCount)
		splitPreviewLabel.SetText(fmt.Sprintf("预览: 将拆分为 %d 个文件\n%s", parts, preview))
	} else {
		points := splitPageRangeEntry.Text
		if points == "" {
			splitPreviewLabel.SetText(fmt.Sprintf("总页数: %d\n请输入拆分点页码", pageCount))
			return
		}
		pageNrs := parseSplitPoints(points)
		if len(pageNrs) == 0 {
			splitPreviewLabel.SetText("请输入有效的拆分点页码")
			return
		}
		sort.Ints(pageNrs)
		preview := generateSplitPreview(baseName, pageNrs, pageCount)
		splitPreviewLabel.SetText(fmt.Sprintf("预览: 将拆分为 %d 个文件\n%s", len(pageNrs)+1, preview))
	}
}

func generateNPageSplitPreview(baseName string, n, totalPages int) string {
	var sb strings.Builder
	start := 1
	partNum := 1
	for start <= totalPages {
		end := start + n - 1
		if end > totalPages {
			end = totalPages
		}
		if sb.Len() > 0 {
			sb.WriteString(fmt.Sprintf("\n%s_%d-%d.pdf", baseName, start, end))
		} else {
			sb.WriteString(fmt.Sprintf("%s_%d-%d.pdf", baseName, start, end))
		}
		start = end + 1
		partNum++
	}
	return sb.String()
}

func generateSplitPreview(baseName string, points []int, totalPages int) string {
	var sb strings.Builder
	prev := 1
	for i, p := range points {
		if i > 0 {
			sb.WriteString("\n")
		}
		if p > prev {
			sb.WriteString(fmt.Sprintf("%s_%d-%d.pdf", baseName, prev, p-1))
		}
		prev = p
	}
	if prev <= totalPages {
		if sb.Len() > 0 {
			sb.WriteString(fmt.Sprintf("\n%s_%d-%d.pdf", baseName, prev, totalPages))
		} else {
			sb.WriteString(fmt.Sprintf("%s_%d-%d.pdf", baseName, prev, totalPages))
		}
	}
	return sb.String()
}

func countPageRanges(input string) int {
	input = strings.ReplaceAll(input, "，", ",")
	parts := strings.Split(input, ",")
	count := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, _ := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, _ := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if start > 0 && end > 0 && end >= start {
					count += end - start + 1
				}
			}
		} else {
			if _, err := strconv.Atoi(part); err == nil {
				count++
			}
		}
	}
	return count
}

func doSplit(win fyne.Window) error {
	if splitInputPath == "" {
		return errors.New("请选择输入文件")
	}
	if splitOutputDir == "" {
		return errors.New("请选择输出目录")
	}

	conf := model.NewDefaultConfiguration()

	if splitModeRadio.Selected == "每 N 页拆分" {
		n, err := strconv.Atoi(splitPageCountEntry.Text)
		if err != nil || n <= 0 {
			return errors.New("请输入有效的页数")
		}
		return api.SplitFile(splitInputPath, splitOutputDir, n, conf)
	} else {
		ranges := splitPageRangeEntry.Text
		if ranges == "" {
			return errors.New("请输入拆分点页码")
		}
		pageNrs := parseSplitPoints(ranges)
		if len(pageNrs) == 0 {
			return errors.New("无效的拆分点页码")
		}
		return api.SplitByPageNrFile(splitInputPath, splitOutputDir, pageNrs, conf)
	}
}

func parseSplitPoints(input string) []int {
	input = strings.ReplaceAll(input, "，", ",")
	parts := strings.Split(input, ",")
	var result []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if n, err := strconv.Atoi(part); err == nil && n > 0 {
			result = append(result, n)
		}
	}
	return result
}

func parsePageNrs(input string) []int {
	input = strings.ReplaceAll(input, "，", ",")
	parts := strings.Split(input, ",")
	var result []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if err1 == nil && err2 == nil && start > 0 && end >= start {
					for i := start; i <= end; i++ {
						result = append(result, i)
					}
				}
			}
		} else {
			if n, err := strconv.Atoi(part); err == nil && n > 0 {
				result = append(result, n)
			}
		}
	}
	return result
}

func createReorderTab(win fyne.Window) *container.TabItem {
	reorderWin = win
	content := container.NewVBox()

	reorderInputLabel = widget.NewLabel("未选择文件")
	selectFileBtn := widget.NewButton("选择文件", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			path := reader.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				dialog.ShowError(fmt.Errorf("请选择PDF文件"), win)
				return
			}
			reorderInputPath = path
			pc, _ := safePageCountFile(path)
			reorderPageCount = pc
			reorderInputLabel.SetText(fmt.Sprintf("%s (共 %d 页)", getFileName(path), pc))
			initReorderPages()
			refreshReorderUI()
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})
	selectFileBtn.Importance = widget.HighImportance

	fileRow := container.NewHBox(
		widget.NewLabel("输入文件:"),
		selectFileBtn,
		reorderInputLabel,
	)
	content.Add(fileRow)
	content.Add(widget.NewSeparator())

	reorderPagesContainer = container.NewVBox()
	content.Add(reorderPagesContainer)

	content.Add(widget.NewSeparator())

	outputEntry := widget.NewEntry()
	defaultName := fmt.Sprintf("reordered-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))
	outputEntry.Text = defaultName

	execBtn := widget.NewButton("开始重排", func() {
		err := doReorder(outputEntry.Text, win)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		dialog.ShowInformation("成功", "PDF 重排完成！", win)
	})
	execBtn.Importance = widget.HighImportance

	selectOutputBtn := widget.NewButton("输出", func() {
		saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			path := writer.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				path += ".pdf"
			}
			outputEntry.SetText(path)
			writer.Close()
			os.Remove(path)
		}, win)
		saveDlg.Resize(fyne.NewSize(800, 600))
		saveDlg.Show()
	})
	selectOutputBtn.Importance = widget.HighImportance

	outputRow := container.NewBorder(nil, nil, selectOutputBtn, execBtn, outputEntry)
	content.Add(outputRow)

	mainContainer := container.NewVScroll(content)

	return container.NewTabItem("重排", mainContainer)
}

func initReorderPages() {
	reorderPages = make([]int, reorderPageCount)
	for i := 0; i < reorderPageCount; i++ {
		reorderPages[i] = i + 1
	}
	reorderSelected = -1
	reorderSecondSelected = -1
	reorderPageWidgets = nil
}

func resetReorder() {
	initReorderPages()
	refreshReorderUI()
}

func selectReorderPage(idx int) {
	if reorderSelected == -1 {
		reorderSelected = idx
	} else if idx == reorderSelected {
		reorderSelected = -1
	} else if reorderSecondSelected == -1 {
		reorderSecondSelected = idx
		reorderPages[reorderSelected], reorderPages[reorderSecondSelected] = reorderPages[reorderSecondSelected], reorderPages[reorderSelected]
		reorderSelected = -1
		reorderSecondSelected = -1
	} else {
		reorderSelected = idx
		reorderSecondSelected = -1
	}
	refreshReorderUI()
}

func refreshReorderUI() {
	if reorderPagesContainer == nil {
		return
	}

	reorderPagesContainer.Objects = nil

	btnWrap := container.NewGridWrap(fyne.NewSize(50, 36))
	reorderPageWidgets = nil
	for i := 0; i < len(reorderPages); i++ {
		page := reorderPages[i]
		idx := i
		btn := widget.NewButton(fmt.Sprintf("%d", page), func() {
			selectReorderPage(idx)
		})
		if i == reorderSelected || i == reorderSecondSelected {
			btn.Importance = widget.HighImportance
		}
		btnWrap.Add(btn)
		reorderPageWidgets = append(reorderPageWidgets, btn)
	}
	reorderPagesContainer.Add(btnWrap)

	opRow := container.NewHBox(
		widget.NewButton("重置", func() { resetReorder() }),
		widget.NewButton("上移", func() {
			if reorderSelected > 0 {
				reorderPages[reorderSelected-1], reorderPages[reorderSelected] = reorderPages[reorderSelected], reorderPages[reorderSelected-1]
				reorderSelected--
				refreshReorderUI()
			}
		}),
		widget.NewButton("下移", func() {
			if reorderSelected >= 0 && reorderSelected < len(reorderPages)-1 {
				reorderPages[reorderSelected+1], reorderPages[reorderSelected] = reorderPages[reorderSelected], reorderPages[reorderSelected+1]
				reorderSelected++
				refreshReorderUI()
			}
		}),
		widget.NewButton("置顶", func() {
			if reorderSelected > 0 {
				page := reorderPages[reorderSelected]
				reorderPages = append([]int{page}, append(reorderPages[:reorderSelected], reorderPages[reorderSelected+1:]...)...)
				reorderSelected = 0
				refreshReorderUI()
			}
		}),
		widget.NewButton("置底", func() {
			if reorderSelected >= 0 && reorderSelected < len(reorderPages)-1 {
				page := reorderPages[reorderSelected]
				reorderPages = append(append(reorderPages[:reorderSelected], reorderPages[reorderSelected+1:]...), page)
				reorderSelected = len(reorderPages) - 1
				refreshReorderUI()
			}
		}),
		widget.NewButton("删除", func() {
			if reorderSelected >= 0 {
				reorderPages = append(reorderPages[:reorderSelected], reorderPages[reorderSelected+1:]...)
				if reorderSelected >= len(reorderPages) {
					reorderSelected = len(reorderPages) - 1
				}
				refreshReorderUI()
			}
		}),
		widget.NewButton("倒序", func() {
			for i, j := 0, len(reorderPages)-1; i < j; i, j = i+1, j-1 {
				reorderPages[i], reorderPages[j] = reorderPages[j], reorderPages[i]
			}
			refreshReorderUI()
		}),
		widget.NewButton("奇偶分离", func() {
			odd := []int{}
			even := []int{}
			for _, p := range reorderPages {
				if p%2 == 1 {
					odd = append(odd, p)
				} else {
					even = append(even, p)
				}
			}
			reorderPages = append(odd, even...)
			refreshReorderUI()
		}),
	)
	reorderPagesContainer.Add(opRow)

	customEntry := widget.NewEntry()
	customEntry.PlaceHolder = "或直接输入顺序，如: 1,3,5,2,4"
	customEntry.OnSubmitted = func(s string) {
		newOrder := parseCustomOrder(s)
		if len(newOrder) > 0 {
			reorderPages = newOrder
			refreshReorderUI()
		}
	}
	reorderPagesContainer.Add(customEntry)

	reorderPagesContainer.Refresh()
}

func parseCustomOrder(input string) []int {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "，", ",")

	parts := strings.Split(input, ",")
	var result []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n > 0 {
			result = append(result, n)
		}
	}
	return result
}

func doReorder(outputPath string, win fyne.Window) error {
	if reorderInputPath == "" {
		return errors.New("请选择输入文件")
	}
	if outputPath == "" {
		return errors.New("请输入输出文件路径")
	}
	if len(reorderPages) == 0 {
		return errors.New("没有页面需要重排")
	}

	conf := model.NewDefaultConfiguration()

	var selectedPages []string
	for _, p := range reorderPages {
		selectedPages = append(selectedPages, fmt.Sprintf("%d", p))
	}

	return api.CollectFile(reorderInputPath, outputPath, selectedPages, conf)
}

func createPageNumTab(win fyne.Window) *container.TabItem {
	content := container.NewVBox()

	pageNumInputLabel = widget.NewLabel("未选择文件")
	selectFileBtn := widget.NewButton("选择文件", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			path := reader.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				dialog.ShowError(fmt.Errorf("请选择PDF文件"), win)
				return
			}
			pageNumInputPath = path
			pc, _ := safePageCountFile(path)
			pageNumPageCount = pc
			pageNumInputLabel.SetText(fmt.Sprintf("%s (共 %d 页)", getFileName(path), pageNumPageCount))
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})
	selectFileBtn.Importance = widget.HighImportance

	fileRow := container.NewHBox(
		widget.NewLabel("输入文件:"),
		selectFileBtn,
		pageNumInputLabel,
	)
	content.Add(fileRow)
	content.Add(widget.NewSeparator())

	pageNumFormatSelect = widget.NewSelect([]string{
		"Page %p of %P",
		"%p/%P",
		"-%p-",
		"%p",
		"第 %p 页",
		"第 %p 页 / 共 %P 页",
	}, func(value string) {
	})
	pageNumFormatSelect.Selected = "%p/%P"

	content.Add(container.NewHBox(
		widget.NewLabel("页码格式:"),
		pageNumFormatSelect,
	))

	content.Add(widget.NewSeparator())

	content.Add(widget.NewLabel("位置:"))
	posHBox := container.NewHBox()
	posRadio := widget.NewRadioGroup([]string{"左下", "居中", "右下", "左上", "居上", "右上"}, func(value string) {
		switch value {
		case "左下":
			pageNumPosition = "bl"
		case "居中":
			pageNumPosition = "bc"
		case "右下":
			pageNumPosition = "br"
		case "左上":
			pageNumPosition = "tl"
		case "居上":
			pageNumPosition = "tc"
		case "右上":
			pageNumPosition = "tr"
		}
	})
	posRadio.Horizontal = true
	posRadio.Selected = "右下"
	posHBox.Add(posRadio)
	content.Add(posHBox)

	pageNumOddEvenCheck = widget.NewCheck("奇偶页位置互换", func(b bool) {})
	pageNumOddEvenCheck.SetChecked(true)
	content.Add(pageNumOddEvenCheck)

	content.Add(widget.NewSeparator())

	fontSizeSelect := widget.NewSelect([]string{"6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35", "36", "37", "38", "39", "40", "41", "42", "43", "44", "45", "46", "47", "48"}, func(value string) {
		if n, err := strconv.Atoi(value); err == nil {
			pageNumFontSize = n
		}
	})
	fontSizeSelect.Selected = "8"

	colorPreview := canvas.NewRectangle(color.Black)
	colorPreview.SetMinSize(fyne.NewSize(24, 24))
	colorBtn := widget.NewButton("选择颜色", func() {
		colorPicker := dialog.NewColorPicker("选择页码颜色", "选择页码颜色", func(c color.Color) {
			if c != nil {
				r, g, b, _ := c.RGBA()
				pageNumColor = fmt.Sprintf("#%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
				colorPreview.FillColor = c
				colorPreview.Refresh()
			}
		}, win)
		colorPicker.Advanced = true
		colorPicker.Show()
	})
	colorBtn.Importance = widget.HighImportance
	content.Add(container.NewHBox(
		widget.NewLabel("字号:"),
		fontSizeSelect,
		widget.NewLabel("   "),
		widget.NewLabel("颜色:"),
		colorPreview,
		colorBtn,
	))

	var marginCustomCheck *widget.Check
	marginCustomEntry := widget.NewEntry()
	marginCustomEntry.PlaceHolder = "边距"
	marginCustomEntry.Hidden = true
	marginCustomEntry.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			pageNumMargin = n
		}
	}

	marginSelect := widget.NewSelect([]string{"6", "8", "10", "12", "14", "16", "18", "20", "24", "30", "36", "42", "48", "54", "60", "66", "72"}, func(value string) {
		if value != "" {
			if n, err := strconv.Atoi(value); err == nil {
				pageNumMargin = n
			}
			marginCustomCheck.SetChecked(false)
			marginCustomEntry.Hidden = true
		}
	})
	marginSelect.Selected = "16"

	marginCustomCheck = widget.NewCheck("自定义", func(checked bool) {
		if checked {
			marginCustomEntry.Hidden = false
			marginSelect.SetSelected("")
		} else {
			marginCustomEntry.Hidden = true
			if marginCustomEntry.Text != "" {
				if n, err := strconv.Atoi(marginCustomEntry.Text); err == nil && n > 0 {
					pageNumMargin = n
				}
			}
		}
	})

	content.Add(container.NewHBox(
		widget.NewLabel("边距:"),
		marginSelect,
		marginCustomCheck,
		marginCustomEntry,
	))

	content.Add(widget.NewSeparator())

	previewBtn := widget.NewButtonWithIcon("预览", theme.VisibilityIcon(), func() {
		showPageNumPreview(win)
	})
	content.Add(container.NewHBox(previewBtn, widget.NewLabel("预览第一页的页码效果（使用当前设置）")))
	content.Add(widget.NewSeparator())

	outputEntry := widget.NewEntry()
	defaultName := fmt.Sprintf("numbered-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))
	outputEntry.Text = defaultName

	execBtn := widget.NewButton("添加页码", func() {
		if outputEntry.Text == "" {
			dialog.ShowError(errors.New("请选择输出文件"), win)
			return
		}
		err := doAddPageNumbers(outputEntry.Text, win)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		dialog.ShowInformation("成功", "页码添加完成！", win)
	})
	execBtn.Importance = widget.HighImportance

	selectOutputBtn := widget.NewButton("输出", func() {
		saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			path := writer.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				path += ".pdf"
			}
			outputEntry.SetText(path)
			writer.Close()
			os.Remove(path)
		}, win)
		saveDlg.Resize(fyne.NewSize(800, 600))
		saveDlg.Show()
	})
	selectOutputBtn.Importance = widget.HighImportance

	outputRow := container.NewBorder(nil, nil, selectOutputBtn, execBtn, outputEntry)
	content.Add(outputRow)

	mainContainer := container.NewVScroll(content)

	return container.NewTabItem("页码", mainContainer)
}

func doAddPageNumbers(outputPath string, win fyne.Window) error {
	if pageNumInputPath == "" {
		return errors.New("请选择输入文件")
	}
	if outputPath == "" {
		return errors.New("请输入输出文件路径")
	}

	if err := ensureChineseFont(); err != nil {
		logD("Warning: Failed to install Chinese font: %v", err)
	}

	format := "Page %p of %P"
	if pageNumFormatSelect != nil && pageNumFormatSelect.Selected != "" {
		format = pageNumFormatSelect.Selected
	}

	conf := model.NewDefaultConfiguration()

	desc := fmt.Sprintf("fontname:WenQuanYiMicroHei, pos:%s, scale:1.0 abs, points:%d, fillc:%s, rot:0, margins:%d", pageNumPosition, pageNumFontSize, pageNumColor, pageNumMargin)

	addFn := func(input, output string, pages []string, onTop bool, format, desc string, conf *model.Configuration) error {
		err := api.AddTextWatermarksFile(input, output, pages, onTop, format, desc, conf)
		if err == nil {
			return nil
		}
		// Fallback: if standard API fails due to PDF validation issues,
		// use low-level approach that bypasses validation.
		return addPageNumbersLowLevel(input, output, pages, onTop, format, desc, conf)
	}

	if pageNumOddEvenCheck != nil && pageNumOddEvenCheck.Checked && pageNumPosition != "bc" {
		evenPos := getOppositePosition(pageNumPosition)
		oddDesc := fmt.Sprintf("fontname:WenQuanYiMicroHei, pos:%s, scale:1.0 abs, points:%d, fillc:%s, rot:0, margins:%d", pageNumPosition, pageNumFontSize, pageNumColor, pageNumMargin)
		evenDesc := fmt.Sprintf("fontname:WenQuanYiMicroHei, pos:%s, scale:1.0 abs, points:%d, fillc:%s, rot:0, margins:%d", evenPos, pageNumFontSize, pageNumColor, pageNumMargin)

		oddPages := getOddPages(pageNumPageCount)
		evenPages := getEvenPages(pageNumPageCount)

		if len(oddPages) > 0 {
			if err := addFn(pageNumInputPath, outputPath, oddPages, true, format, oddDesc, conf); err != nil {
				return err
			}
		}
		if len(evenPages) > 0 {
			if len(oddPages) > 0 {
				tempFile := outputPath + ".temp.pdf"
				os.Rename(outputPath, tempFile)
				err := addFn(tempFile, outputPath, evenPages, true, format, evenDesc, conf)
				os.Remove(tempFile)
				if err != nil {
					return err
				}
			} else {
				if err := addFn(pageNumInputPath, outputPath, evenPages, true, format, evenDesc, conf); err != nil {
					return err
				}
			}
		}
	} else {
		if err := addFn(pageNumInputPath, outputPath, nil, true, format, desc, conf); err != nil {
			return err
		}
	}

	return nil
}

func safePageCountFile(path string) (int, error) {
	count, err := api.PageCountFile(path)
	if err == nil {
		return count, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	ctx, err := pdfcpu.Read(f, conf)
	if err != nil {
		return 0, err
	}
	if err := ctx.XRefTable.EnsurePageCount(); err != nil {
		return 0, err
	}
	return ctx.PageCount, nil
}

func fixBadAnnotations(ctx *model.Context) {
	for i := 1; i <= ctx.PageCount; i++ {
		pageDict, _, _, _ := ctx.XRefTable.PageDict(i, false)
		if pageDict == nil {
			continue
		}
		annots := pageDict.ArrayEntry("Annots")
		for _, a := range annots {
			annotDict, _ := ctx.XRefTable.DereferenceDict(a)
			if annotDict == nil {
				continue
			}
			bs := annotDict["BS"]
			if bs != nil {
				if _, ok := bs.(types.Dict); !ok {
					annotDict["BS"] = nil
				}
			}
		}
	}
}

func addPageNumbersLowLevel(inputPath, outputPath string, selectedPages []string, onTop bool, format, desc string, conf *model.Configuration) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return err
	}

	ctx, err := pdfcpu.Read(f, conf)
	f.Close()
	if err != nil {
		return err
	}
	if err := ctx.XRefTable.EnsurePageCount(); err != nil {
		return err
	}

	fixBadAnnotations(ctx)
	conf.Cmd = model.ADDWATERMARKS
	conf.OptimizeDuplicateContentStreams = false

	unit := types.POINTS
	if conf != nil {
		unit = conf.Unit
	}
	wm, err := api.TextWatermark(format, desc, onTop, false, unit)
	if err != nil {
		return err
	}

	var pages types.IntSet
	if len(selectedPages) > 0 {
		pages = types.IntSet{}
		for _, s := range selectedPages {
			n, err := strconv.Atoi(s)
			if err == nil && n > 0 && n <= ctx.PageCount {
				pages[n] = true
			}
		}
	}

	if err := pdfcpu.AddWatermarks(ctx, pages, wm); err != nil {
		return err
	}

	return api.WriteContextFile(ctx, outputPath)
}

func getOddPages(pageCount int) []string {
	var pages []string
	for i := 1; i <= pageCount; i++ {
		if i%2 == 1 {
			pages = append(pages, fmt.Sprintf("%d", i))
		}
	}
	return pages
}

func getEvenPages(pageCount int) []string {
	var pages []string
	for i := 1; i <= pageCount; i++ {
		if i%2 == 0 {
			pages = append(pages, fmt.Sprintf("%d", i))
		}
	}
	return pages
}

func getOppositePosition(pos string) string {
	switch pos {
	case "bl":
		return "br"
	case "br":
		return "bl"
	case "tl":
		return "tr"
	case "tr":
		return "tl"
	case "tc":
		return "tc"
	case "bc":
		return "bc"
	}
	return pos
}

func showPageNumPreview(win fyne.Window) {
	if pageNumInputPath == "" {
		dialog.ShowError(errors.New("请先选择输入文件"), win)
		return
	}

	format := "Page %p of %P"
	if pageNumFormatSelect != nil && pageNumFormatSelect.Selected != "" {
		format = pageNumFormatSelect.Selected
	}
	pos := pageNumPosition
	fontSize := pageNumFontSize
	color := pageNumColor
	margin := pageNumMargin

	loading := dialog.NewCustom("页码预览", "",
		container.NewVBox(widget.NewLabel("正在生成预览...")),
		win)
	loading.Show()

	go func() {
		if err := ensureChineseFont(); err != nil {
			logD("Warning: Failed to install Chinese font: %v", err)
		}

		desc := fmt.Sprintf("fontname:WenQuanYiMicroHei, pos:%s, scale:1.0 abs, points:%d, fillc:%s, rot:0, margins:%d",
			pos, fontSize, color, margin)

		conf := model.NewDefaultConfiguration()

		tmpDir, err := os.MkdirTemp("", "gtpdf-preview-*")
		if err != nil {
			fyne.Do(func() {
				loading.Hide()
				dialog.ShowError(fmt.Errorf("创建临时目录失败: %v", err), win)
			})
			return
		}
		defer os.RemoveAll(tmpDir)

		tmpPage := filepath.Join(tmpDir, "page.pdf")
		if err := api.TrimFile(pageNumInputPath, tmpPage, []string{"1"}, conf); err != nil {
			fyne.Do(func() {
				loading.Hide()
				dialog.ShowError(fmt.Errorf("提取页面失败: %v", err), win)
			})
			return
		}

		tmpWM := filepath.Join(tmpDir, "wm.pdf")
		addFn := func(input, output string, pages []string, onTop bool, format, desc string, conf *model.Configuration) error {
			err := api.AddTextWatermarksFile(input, output, pages, onTop, format, desc, conf)
			if err == nil {
				return nil
			}
			return addPageNumbersLowLevel(input, output, pages, onTop, format, desc, conf)
		}
		if err := addFn(tmpPage, tmpWM, nil, true, format, desc, conf); err != nil {
			fyne.Do(func() {
				loading.Hide()
				dialog.ShowError(fmt.Errorf("生成预览失败: %v", err), win)
			})
			return
		}

		doc, err := pdfium_plus.OpenDocument(tmpWM)
		if err != nil {
			fyne.Do(func() {
				loading.Hide()
				dialog.ShowError(fmt.Errorf("打开预览文件失败: %v", err), win)
			})
			return
		}
		defer doc.Close()

		img, cleanup, err := doc.RenderPage(0, 96, false)
		if err != nil {
			fyne.Do(func() {
				loading.Hide()
				dialog.ShowError(fmt.Errorf("渲染预览失败: %v", err), win)
			})
			return
		}
		defer cleanup()

		fyne.Do(func() {
			loading.Hide()
			previewImg := canvas.NewImageFromImage(img)
			previewImg.FillMode = canvas.ImageFillContain
			previewImg.SetMinSize(fyne.NewSize(400, 500))
			scroll := container.NewVScroll(previewImg)
			scroll.SetMinSize(fyne.NewSize(700, 800))
			dialog.ShowCustom("页码预览 - 第1页", "关闭", scroll, win)
		})
	}()
}

func createRotateTab(win fyne.Window) *container.TabItem {
	content := container.NewVBox()

	rotateInputLabel = widget.NewLabel("未选择文件")
	selectFileBtn := widget.NewButton("选择文件", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			path := reader.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				dialog.ShowError(fmt.Errorf("请选择PDF文件"), win)
				return
			}
			rotateInputPath = path
			pc, _ := safePageCountFile(path)
			rotatePageCount = pc
			rotateInputLabel.SetText(fmt.Sprintf("%s (共 %d 页)", getFileName(path), pc))
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})
	selectFileBtn.Importance = widget.HighImportance

	fileRow := container.NewHBox(
		widget.NewLabel("输入文件:"),
		selectFileBtn,
		rotateInputLabel,
	)
	content.Add(fileRow)
	content.Add(widget.NewSeparator())

	content.Add(widget.NewLabel("旋转角度:"))

	angleRadio := widget.NewRadioGroup([]string{"顺时针 90°", "逆时针 90°", "顺时针 180°", "逆时针 180°"}, func(value string) {
		switch value {
		case "顺时针 90°":
			rotateAngle = 90
		case "逆时针 90°":
			rotateAngle = -90
		case "顺时针 180°":
			rotateAngle = 180
		case "逆时针 180°":
			rotateAngle = -180
		}
	})
	angleRadio.Selected = "顺时针 90°"
	content.Add(angleRadio)

	content.Add(widget.NewSeparator())

	content.Add(widget.NewLabel("应用范围:"))
	rotateRangeRadio = widget.NewRadioGroup([]string{"全部页面", "奇数页面", "偶数页面", "自定义"}, func(value string) {
		if value == "自定义" {
			rotateRangeEntry.Hidden = false
		} else {
			rotateRangeEntry.Hidden = true
		}
	})
	rotateRangeRadio.Selected = "全部页面"
	content.Add(rotateRangeRadio)

	rotateRangeEntry = widget.NewEntry()
	rotateRangeEntry.PlaceHolder = "如: 1-3, 5, 7-10"
	rotateRangeEntry.Hidden = true
	content.Add(rotateRangeEntry)

	content.Add(widget.NewSeparator())

	outputEntry := widget.NewEntry()
	defaultName := fmt.Sprintf("rotated-%s.pdf", time.Now().Format("2006-01-02_15-04-05"))
	outputEntry.Text = defaultName

	execBtn := widget.NewButton("开始旋转", func() {
		err := doRotate(outputEntry.Text, win)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		dialog.ShowInformation("成功", "PDF 旋转完成！", win)
	})
	execBtn.Importance = widget.HighImportance

	selectOutputBtn := widget.NewButton("输出", func() {
		saveDlg := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			path := writer.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
				path += ".pdf"
			}
			outputEntry.SetText(path)
			writer.Close()
			os.Remove(path)
		}, win)
		saveDlg.Resize(fyne.NewSize(800, 600))
		saveDlg.Show()
	})
	selectOutputBtn.Importance = widget.HighImportance

	outputRow := container.NewBorder(nil, nil, selectOutputBtn, execBtn, outputEntry)
	content.Add(outputRow)

	mainContainer := container.NewVScroll(content)

	return container.NewTabItem("旋转", mainContainer)
}

func doRotate(outputPath string, win fyne.Window) error {
	if rotateInputPath == "" {
		return errors.New("请选择输入文件")
	}
	if outputPath == "" {
		return errors.New("请输入输出文件路径")
	}

	conf := model.NewDefaultConfiguration()

	var selectedPages []string

	switch rotateRangeRadio.Selected {
	case "全部页面":
		selectedPages = nil
	case "奇数页面":
		selectedPages = getOddPages(rotatePageCount)
	case "偶数页面":
		selectedPages = getEvenPages(rotatePageCount)
	case "自定义":
		selectedPages = parsePageSelection(rotateRangeEntry.Text)
	}

	return api.RotateFile(rotateInputPath, outputPath, rotateAngle, selectedPages, conf)
}

func parsePageSelection(input string) []string {
	if input == "" {
		return nil
	}
	input = strings.ReplaceAll(input, "，", ",")
	parts := strings.Split(input, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func getFileName(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if len(name) > 30 {
		name = name[:27] + "..."
	}
	return name
}

func parsePageInput(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("请输入页码")
	}

	input = strings.ReplaceAll(input, "，", ",")

	parts := strings.Split(input, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("无效的页码格式: %s", part)
			}
			start := strings.TrimSpace(rangeParts[0])
			end := strings.TrimSpace(rangeParts[1])

			if start != "" {
				if _, err := strconv.Atoi(start); err != nil {
					return nil, fmt.Errorf("无效的起始页码: %s", start)
				}
			}
			if end != "" {
				if _, err := strconv.Atoi(end); err != nil {
					return nil, fmt.Errorf("无效的结束页码: %s", end)
				}
			}
			result = append(result, part)
		} else {
			if _, err := strconv.Atoi(part); err != nil {
				return nil, fmt.Errorf("无效的页码: %s", part)
			}
			result = append(result, part)
		}
	}

	if len(result) == 0 {
		return nil, errors.New("请输入有效的页码")
	}

	return result, nil
}

func mergePDFs(outputPath string, win fyne.Window) error {
	validCount := 0
	validIndex := -1
	for i := 0; i < len(pdfFiles); i++ {
		if pdfFiles[i].path != "" {
			validCount++
			validIndex = i
		}
	}

	if validCount < 1 {
		return errors.New("请至少选择1个PDF文件")
	}

	if outputPath == "" {
		return errors.New("请填写输出文件路径")
	}

	conf := model.NewDefaultConfiguration()
	conf.CreateBookmarks = false

	if validCount == 1 {
		pageInput := pdfFiles[validIndex].pageEntry.Text

		pageSel, err := parsePageInput(pageInput)
		if err != nil {
			return fmt.Errorf("页码格式错误: %v\n\n示例: 1-3,5,9-11", err)
		}

		err = api.TrimFile(pdfFiles[validIndex].path, outputPath, pageSel, conf)
		if err != nil {
			return fmt.Errorf("提取页面失败 (%s): %v", pdfFiles[validIndex].path, err)
		}

		return nil
	}

	var inputFiles []string
	var tempFiles []string
	defer func() {
		for _, f := range tempFiles {
			os.Remove(f)
		}
	}()

	for i := 0; i < len(pdfFiles); i++ {
		if pdfFiles[i].path == "" {
			continue
		}

		pageInput := pdfFiles[i].pageEntry.Text

		if pageInput != "" {
			pageSel, err := parsePageInput(pageInput)
			if err != nil {
				return fmt.Errorf("文件 %d 页码格式错误: %v\n\n示例: 1-3,5,9-11", i+1, err)
			}

			tempFile := fmt.Sprintf("./temp_%d.pdf", i)

			err = api.TrimFile(pdfFiles[i].path, tempFile, pageSel, conf)
			if err != nil {
				return fmt.Errorf("提取页面失败 (%s): %v", pdfFiles[i].path, err)
			}

			tempFiles = append(tempFiles, tempFile)
			inputFiles = append(inputFiles, tempFile)
		} else {
			inputFiles = append(inputFiles, pdfFiles[i].path)
		}
	}

	err := api.MergeCreateFile(inputFiles, outputPath, false, conf)
	if err != nil {
		return fmt.Errorf("合并失败: %v", err)
	}

	return nil
}

func init() {
	initAnnotSettings()
	pdfFiles = make([]PDFFile, 5)
	for i := range pdfFiles {
		pdfFiles[i] = PDFFile{
			pathLabel: widget.NewLabel("未选择文件"),
			pageEntry: widget.NewEntry(),
		}
	}
}

func getPDFCPUFontDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "pdfcpu", "fonts"), nil
}

func ensureChineseFont() error {
	font.LoadUserFonts()

	toInstall := []string{}

	if !font.IsUserFont("WenQuanYiMicroHei") {
		toInstall = append(toInstall, "wqy-microhei.ttc")
	}
	if !font.IsUserFont("SourceHanSansSC-Regular") {
		toInstall = append(toInstall, "SourceHanSansSC-Regular.otf")
	}

	if len(toInstall) == 0 {
		return nil
	}

	tempDir, err := os.MkdirTemp("", "gtpdf-font-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	var paths []string
	for _, name := range toInstall {
		tempPath := filepath.Join(tempDir, name)
		f, err := fontFS.Open(name)
		if err != nil {
			return fmt.Errorf("打开字体失败 %s: %v", name, err)
		}
		out, err := os.Create(tempPath)
		if err != nil {
			f.Close()
			return err
		}
		_, err = io.Copy(out, f)
		f.Close()
		out.Close()
		if err != nil {
			return err
		}
		paths = append(paths, tempPath)
	}

	if err := api.InstallFonts(paths); err != nil {
		return fmt.Errorf("安装字体失败: %v", err)
	}

	return nil
}
