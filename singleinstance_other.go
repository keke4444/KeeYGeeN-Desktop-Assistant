//go:build !windows

package main

// ensureSingleInstance 在非 Windows 平台暂不做单实例限制，始终允许启动。
func ensureSingleInstance() bool { return true }
