package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

// appTitle 是窗口标题，也是安装后桌面图标显示的名称。
var appTitle = "KeeYGeeN桌面助手"

// appCtx 保存 Wails 上下文，供托盘菜单退出程序时使用。
var appCtx context.Context

func main() {
	if !ensureSingleInstance() {
		return
	}
	app, err := NewApp()
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     appTitle,
		Width:     1150,
		Height:    720,
		MinWidth:  860,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 40, G: 22, B: 32, A: 1},
		OnStartup: func(ctx context.Context) {
			appCtx = ctx
			app.startup(ctx)
			// 在任务栏通知区域显示小图标
			startTray()
		},
		OnShutdown: func(ctx context.Context) {
			stopTray()
		},
		OnBeforeClose: func(ctx context.Context) bool {
			// 勾选「关闭时隐藏到托盘」后，点 × 只隐藏窗口，程序留在托盘
			if app.hideOnClose() {
				runtime.WindowHide(ctx)
				return true
			}
			return false
		},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
