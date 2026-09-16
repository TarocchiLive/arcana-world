// Package overlay 管理原生桌面文字浮层的配置、控制连接和子进程。
//
// # 原生行为约定
//
// 浮层以显示器完整边界而非工作区定位；Dock、桌面面板和前台窗口
// 不应改变锚点。仅更新文本时不调整窗口尺寸，所有更新均不激活窗口。
// 原生后端位于 native 子包，负责 UI 线程、显示器标识和绘制资源；
// 本包保持纯 Go，核心控制流无需图形库即可管理浮层。
//
// # 平台能力
//
// macOS 使用不激活的 AppKit 面板；Windows 使用 Win32 分层窗口。
// Linux 同时要求原生 Wayland 会话和 zwlr_layer_shell_v1 协议。
// 二者是独立能力：正常运行的 GNOME/Mutter Wayland 桌面仍不提供
// layer-shell。安装协议 XML、启用 Wayland 或更换登录管理器，
// 都无法补上合成器未实现的协议。
//
// KDE/KWin 和 niri 提供 layer-shell。支持 GNOME 需要单独集成 Shell；
// 改用普通应用窗口会破坏桌面浮层约定。当前明确不支持 X11，
// 也不回退到 XWayland。所有平台均不保证浮层覆盖安全桌面、锁屏
// 或独占全屏。
//
// # 验证边界
//
// 原生 macOS 已验证九宫格定位、动态尺寸/内边距/字体及进程回收。
// 原型阶段用户在 Parallels 的 Debian 13 ARM64 中测试了完整的
// KDE/KWin 6.3.6 和 niri 26.04 会话；受管版本另在嵌套 KWin/niri
// 中验证了动态布局、分数缩放、控制断连和错误传播。
// GNOME 48.7 桌面正常，只有浮层因 Mutter 缺少 layer-shell 而失败。
// Windows 11 ARM64 的 Parallels 原生用户会话已通过 Unix socket、
// 私有 DACL、200% DPI 下的九宫格、字体/内边距及不抢焦点验证；
// Windows AMD64 本轮仅交叉编译，早期运行验证使用 Wine 10 加 xcompmgr。
// 这些结果不代表物理 GPU、直接扫描输出、混合 DPI 多显示器热插拔、
// 重连或睡眠唤醒已经验证。
package overlay
