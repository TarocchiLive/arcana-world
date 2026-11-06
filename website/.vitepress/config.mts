import { defineConfig, type DefaultTheme } from 'vitepress';

// 发布到 https://tarocchilive.github.io/arcana-world/。
const base = '/arcana-world/';
const repoURL = 'https://github.com/TarocchiLive/arcana-world';

// 侧边栏分组：[路径, 标题]。
const groups: { text: string; items: [string, string][] }[] = [
	{
		text: '入门',
		items: [
			['guide/introduction', '了解 Arcana World'],
			['guide/install', '安装与升级'],
			['guide/start-web', '快速上手：网页面板'],
			['guide/start-tui', '快速上手：终端界面'],
			['guide/terminal', '终端入门'],
		],
	},
	{
		text: '功能',
		items: [
			['features/room', '直播间设置'],
			['features/obs', 'OBS 与推流'],
			['features/chat', '弹幕与观众管理'],
			['features/blocklist', '屏蔽词'],
			['features/overlay', '弹幕浮层'],
			['features/tts', '语音播报'],
			['features/templates', '自定义浮层与播报文字'],
			['features/websocket', 'WebSocket 弹幕广播'],
		],
	},
	{
		text: '参考',
		items: [
			['reference/cli', '命令行参数'],
			['reference/data', '数据目录与重置'],
			['reference/troubleshooting', '常见问题'],
		],
	},
	{
		text: '开发',
		items: [['development/', '参与开发']],
	},
];

const sidebar: DefaultTheme.SidebarItem[] = groups.map((group) => ({
	text: group.text,
	items: group.items.map(([path, text]) => ({ text, link: `/${path}` })),
}));

const nav: DefaultTheme.NavItem[] = [
	{ text: '指南', link: '/guide/introduction', activeMatch: '^/guide/' },
	{ text: '功能', link: '/features/overlay', activeMatch: '^/features/' },
	{ text: '参考', link: '/reference/troubleshooting', activeMatch: '^/reference/' },
	{ text: '下载', link: `${repoURL}/releases/latest` },
];

export default defineConfig({
	base,
	srcDir: 'src',
	outDir: 'dist',
	cleanUrls: true,
	lastUpdated: true,
	title: 'Arcana World',
	head: [
		['link', { rel: 'icon', type: 'image/webp', href: `${base}images/arcana-world-icon.webp` }],
		['meta', { name: 'theme-color', content: '#6c4ee0' }],
	],
	lang: 'zh-CN',
	description: '跨平台的 bilibili 直播姬 + 弹幕姬：扫码登录、连接 OBS 开播，弹幕浮层与语音播报。',
	themeConfig: {
		logo: '/images/arcana-world-icon.webp',
		socialLinks: [{ icon: 'github', link: repoURL }],
		nav,
		sidebar,
		editLink: { pattern: `${repoURL}/edit/main/website/src/:path`, text: '在 GitHub 上编辑此页' },
		lastUpdated: { text: '最后更新' },
		outline: { label: '本页目录', level: [2, 3] },
		docFooter: { prev: '上一页', next: '下一页' },
		darkModeSwitchLabel: '外观',
		lightModeSwitchTitle: '切换到浅色模式',
		darkModeSwitchTitle: '切换到深色模式',
		sidebarMenuLabel: '菜单',
		returnToTopLabel: '返回顶部',
		notFound: { title: '页面未找到', quote: '', linkLabel: '返回首页', linkText: '返回首页' },
		footer: { message: '基于 GPL-3.0-only 许可发布', copyright: 'Arcana World' },
		search: {
			provider: 'local',
			options: {
				translations: {
					button: { buttonText: '搜索', buttonAriaLabel: '搜索' },
					modal: {
						displayDetails: '显示详情',
						resetButtonTitle: '清除查询',
						backButtonTitle: '关闭搜索',
						noResultsText: '没有找到结果',
						footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' },
					},
				},
			},
		},
	},
});
