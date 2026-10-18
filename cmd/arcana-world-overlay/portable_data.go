package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// configurePortableData 必须先于显示器枚举及图形后端初始化执行。
func configurePortableData() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate portable overlay data: %w", err)
	}
	directory := filepath.Dir(executable)
	if _, explicit := os.LookupEnv("XKB_CONFIG_ROOT"); !explicit {
		root := filepath.Join(directory, "share", "X11", "xkb")
		info, err := os.Stat(root)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect bundled keyboard data %q: %w", root, err)
		}
		if err == nil && info.IsDir() {
			if err := os.Setenv("XKB_CONFIG_ROOT", root); err != nil {
				return fmt.Errorf("set bundled keyboard data: %w", err)
			}
		}
	}
	// 任一显式 fontconfig 设置（包括空值）都保留整套用户配置，避免混用配置来源。
	_, explicitFile := os.LookupEnv("FONTCONFIG_FILE")
	_, explicitPath := os.LookupEnv("FONTCONFIG_PATH")
	if !explicitFile && !explicitPath {
		root := filepath.Join(directory, "etc", "fonts")
		config := filepath.Join(root, "fonts.conf")
		info, err := os.Stat(config)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect bundled font configuration %q: %w", config, err)
		}
		if err == nil && info.Mode().IsRegular() {
			if err := os.Setenv("FONTCONFIG_FILE", config); err != nil {
				return fmt.Errorf("set bundled font configuration: %w", err)
			}
			if err := os.Setenv("FONTCONFIG_PATH", root); err != nil {
				return fmt.Errorf("set bundled font configuration directory: %w", err)
			}
		}
	}
	return nil
}
