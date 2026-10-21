//go:build windows

// Package shellicon 通过 Win32 Shell API 取出 Windows 资源管理器使用的真实文件图标，
// 编码为 PNG data URL 供前端 <img> 直接显示。
package shellicon

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procGetProcAddress     = kernel32.NewProc("GetProcAddress")
	procSHGetFileInfoW     = shell32.NewProc("SHGetFileInfoW")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procDrawIconEx         = user32.NewProc("DrawIconEx")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
)

const (
	shgfiIcon         = 0x000000100
	shgfiLargeIcon    = 0x000000000
	shgfiSysIconIndex = 0x000004000
	dibRGBColors      = 0x00
	biRGB             = 0
	iconSize          = 48 // 取系统「超大图标」，放大后仍然清晰
	shilExtraLarge    = 2  // SHIL_EXTRALARGE = 48x48
	ildTransparent    = 1  // ILD_TRANSPARENT
	folderCacheKey    = "__folder__"
	folderSamplePath  = `C:\Windows`
)

// guid 对应 Windows 的 GUID 结构体，内存布局与 REFIID 一致。
type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

// IID_IImageList: {46EB5926-582E-4017-9FDF-E8998DAA0950}
var iidIImageList = guid{0x46EB5926, 0x582E, 0x4017, [8]byte{0x9F, 0xDF, 0xE8, 0x99, 0x8D, 0xAA, 0x09, 0x50}}

var (
	imageListOnce sync.Once
	imageListProc uintptr
)

type shFileInfo struct {
	hIcon         syscall.Handle
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
	bmiColors [4]uint32
}

var (
	iconCache sync.Map // key -> string
	pathExts  = map[string]bool{
		".exe": true, ".lnk": true, ".url": true, ".ico": true,
		".msi": true, ".bat": true, ".cmd": true, ".appref-ms": true,
	}
)

// IconFor 返回文件或目录的图标 data URL；失败返回空字符串（前端回退到内置矢量图标）。
func IconFor(path string, isDir bool) string {
	key := cacheKey(path, isDir)
	if v, ok := iconCache.Load(key); ok {
		return v.(string)
	}
	url := extract(path)
	iconCache.Store(key, url)
	return url
}

// FolderIcon 返回通用文件夹图标。
func FolderIcon() string {
	return IconFor(folderSamplePath, true)
}

func cacheKey(path string, isDir bool) string {
	if isDir {
		return folderCacheKey
	}
	ext := strings.ToLower(filepath.Ext(path))
	if pathExts[ext] {
		return "path|" + strings.ToLower(path)
	}
	if ext == "" {
		return "noext"
	}
	return "ext|" + ext
}

func extract(path string) string {
	// 优先从系统「超大图标」列表取 48x48；失败再回退 SHGetFileInfo 的 32x32
	if idx, err := sysIconIndex(path); err == nil {
		if hIcon, err := imageListIcon(idx); err == nil && hIcon != 0 {
			if pngBytes, err := hiconToPNG(hIcon, iconSize); err == nil && len(pngBytes) > 0 {
				procDestroyIcon.Call(uintptr(hIcon))
				return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
			}
			procDestroyIcon.Call(uintptr(hIcon))
		}
	}

	hIcon, err := basicIcon(path)
	if err != nil || hIcon == 0 {
		return ""
	}
	defer procDestroyIcon.Call(uintptr(hIcon))

	pngBytes, err := hiconToPNG(hIcon, 32)
	if err != nil || len(pngBytes) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
}

// sysIconIndex 取文件在系统图标列表中的序号（不返回图标句柄）。
func sysIconIndex(path string) (int32, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var info shFileInfo
	ret, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		uintptr(shgfiSysIconIndex|shgfiLargeIcon),
	)
	if ret == 0 {
		return 0, fmt.Errorf("SHGetFileInfo 取图标序号失败: %v", callErr)
	}
	return info.iIcon, nil
}

// basicIcon 走 SHGetFileInfo 取系统大图标（32x32）。
func basicIcon(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var info shFileInfo
	ret, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		uintptr(shgfiIcon|shgfiLargeIcon),
	)
	if ret == 0 || info.hIcon == 0 {
		return 0, fmt.Errorf("SHGetFileInfo 失败: %v", callErr)
	}
	return info.hIcon, nil
}

// shGetImageListAddr 取 SHGetImageList 的地址。该 API 只按序号 727 导出，
// 不能用 NewProc("SHGetImageList") 按名字查找。
func shGetImageListAddr() uintptr {
	imageListOnce.Do(func() {
		name, err := syscall.UTF16PtrFromString("shell32.dll")
		if err != nil {
			return
		}
		h, _, _ := procGetModuleHandleW.Call(uintptr(unsafe.Pointer(name)))
		if h == 0 {
			return
		}
		addr, _, _ := procGetProcAddress.Call(h, 727)
		imageListProc = addr
	})
	return imageListProc
}

// imageListIcon 从系统「超大图标」列表取指定序号的 48x48 图标。
func imageListIcon(index int32) (syscall.Handle, error) {
	addr := shGetImageListAddr()
	if addr == 0 {
		return 0, fmt.Errorf("SHGetImageList 不可用")
	}

	var list unsafe.Pointer
	// SHGetImageList(iImageList int, riid REFIID, ppvObj *unsafe.Pointer)
	r, _, _ := syscall.SyscallN(addr,
		uintptr(shilExtraLarge),
		uintptr(unsafe.Pointer(&iidIImageList)),
		uintptr(unsafe.Pointer(&list)),
	)
	if r != 0 || list == nil {
		return 0, fmt.Errorf("SHGetImageList 失败: 0x%x", r)
	}
	defer comRelease(list)

	var hIcon syscall.Handle
	// IImageList 继承 IUnknown：前 3 个是 QueryInterface/AddRef/Release，
	// 之后第 7 个方法是 GetIcon，虚表下标为 10。
	getIcon := comMethod(list, 10)
	r, _, _ = syscall.SyscallN(getIcon,
		uintptr(list), uintptr(index), uintptr(ildTransparent),
		uintptr(unsafe.Pointer(&hIcon)))
	if r != 0 || hIcon == 0 {
		return 0, fmt.Errorf("IImageList.GetIcon 失败: 0x%x", r)
	}
	return hIcon, nil
}

// comMethod 取 COM 接口虚表里第 i 个方法的地址。
func comMethod(obj unsafe.Pointer, i uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(vtbl, i*unsafe.Sizeof(uintptr(0))))
}

// comRelease 调用 COM 对象的 Release（虚表下标 2）。
func comRelease(obj unsafe.Pointer) {
	_, _, _ = syscall.SyscallN(comMethod(obj, 2), uintptr(obj))
}

func hiconToPNG(hIcon syscall.Handle, size int) ([]byte, error) {
	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return nil, fmt.Errorf("GetDC 失败")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(hdcMem)

	bi := bitmapInfo{}
	bi.bmiHeader.biSize = uint32(unsafe.Sizeof(bi.bmiHeader))
	bi.bmiHeader.biWidth = int32(size)
	bi.bmiHeader.biHeight = -int32(size) // 负数 = 自上而下
	bi.bmiHeader.biPlanes = 1
	bi.bmiHeader.biBitCount = 32
	bi.bmiHeader.biCompression = biRGB

	var bits unsafe.Pointer
	hbm, _, _ := procCreateDIBSection.Call(
		hdcScreen,
		uintptr(unsafe.Pointer(&bi)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbm == 0 || bits == nil {
		return nil, fmt.Errorf("CreateDIBSection 失败")
	}
	defer procDeleteObject.Call(hbm)

	old, _, _ := procSelectObject.Call(hdcMem, hbm)
	defer procSelectObject.Call(hdcMem, old)

	buf := unsafe.Slice((*byte)(bits), size*size*4)
	for i := range buf {
		buf[i] = 0
	}

	if r, _, _ := procDrawIconEx.Call(hdcMem, 0, 0, uintptr(hIcon),
		uintptr(size), uintptr(size), 0, 0, 3); r == 0 {
		return nil, fmt.Errorf("DrawIconEx 失败")
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < size*size; i++ {
		b := uint32(buf[i*4+0])
		g := uint32(buf[i*4+1])
		r := uint32(buf[i*4+2])
		a := uint32(buf[i*4+3])
		// DrawIconEx 输出为预乘 alpha，转回直通 alpha 以免颜色发暗
		if a > 0 && a < 255 {
			r = min32(255, r*255/a)
			g = min32(255, g*255/a)
			b = min32(255, b*255/a)
		}
		img.Pix[i*4+0] = uint8(r)
		img.Pix[i*4+1] = uint8(g)
		img.Pix[i*4+2] = uint8(b)
		img.Pix[i*4+3] = uint8(a)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
