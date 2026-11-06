<script setup lang="ts">
import { withBase } from 'vitepress';

// 首页“三步开播”与平台下载区文案。
const t = {
	title: '三步开始直播',
	lead: '首次完成设置后，每次开播只需确认一次。',
	steps: [
		{ title: '扫码登录', text: '使用哔哩哔哩 App 扫码，自动载入直播间。' },
		{ title: '连接 OBS', text: '在 OBS 中启用 WebSocket 服务器，填入密码，之后自动连接。' },
		{ title: '开播', text: '确认后推流地址自动写入 OBS 并开始推流。停播同样一步完成。' },
	],
	starts: [
		{ text: '网页面板快速上手', link: '/guide/start-web', theme: 'brand' },
		{ text: '终端界面快速上手', link: '/guide/start-tui', theme: 'alt' },
	],
	platformsTitle: '支持的平台',
	platforms: [
		{ name: 'Windows', detail: 'x64 · ARM64', how: '压缩包或 Scoop' },
		{ name: 'macOS', detail: 'Apple 芯片', how: '压缩包' },
		{ name: 'Linux', detail: 'x64 · ARM64', how: '压缩包、APT 或 Nix' },
	],
	install: { text: '查看安装方法', link: '/guide/install' },
};
</script>

<template>
	<section class="home-steps">
		<h2 class="section-title">{{ t.title }}</h2>
		<p class="section-lead">{{ t.lead }}</p>
		<ol class="steps">
			<li v-for="(step, i) in t.steps" :key="step.title" class="step">
				<span class="num">{{ i + 1 }}</span>
				<h3>{{ step.title }}</h3>
				<p>{{ step.text }}</p>
			</li>
		</ol>
		<div class="cta">
			<a v-for="s in t.starts" :key="s.link" class="button" :class="s.theme" :href="withBase(s.link)">{{ s.text }}</a>
		</div>

		<h2 class="section-title platforms-title">{{ t.platformsTitle }}</h2>
		<div class="platforms">
			<div v-for="p in t.platforms" :key="p.name" class="platform">
				<strong>{{ p.name }}</strong>
				<span class="detail">{{ p.detail }}</span>
				<span class="how">{{ p.how }}</span>
			</div>
		</div>
		<div class="cta">
			<a class="button alt" :href="withBase(t.install.link)">{{ t.install.text }}</a>
		</div>
	</section>
</template>

<style scoped>
.home-steps {
	margin: 0 auto;
	padding: 72px 24px 24px;
	max-width: 1152px;
}

.section-title {
	margin: 0;
	text-align: center;
	font-size: 28px;
	font-weight: 700;
	line-height: 1.3;
	letter-spacing: -0.01em;
}

.section-lead {
	margin: 12px auto 0;
	max-width: 560px;
	text-align: center;
	color: var(--vp-c-text-2);
	line-height: 1.7;
}

.steps {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
	gap: 16px;
	margin: 36px 0 0;
	padding: 0;
	list-style: none;
}

.step {
	position: relative;
	padding: 28px 24px 24px;
	border-radius: 14px;
	border: 1px solid var(--vp-c-divider);
	background: var(--vp-c-bg-soft);
}

.num {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 32px;
	height: 32px;
	border-radius: 50%;
	background: var(--vp-c-brand-soft);
	color: var(--vp-c-brand-1);
	font-weight: 700;
}

.step h3 {
	margin: 16px 0 8px;
	font-size: 17px;
	font-weight: 600;
}

.step p {
	margin: 0;
	color: var(--vp-c-text-2);
	font-size: 14px;
	line-height: 1.7;
}

.cta {
	display: flex;
	flex-wrap: wrap;
	gap: 12px;
	justify-content: center;
	margin-top: 28px;
}

.button {
	display: inline-block;
	padding: 0 22px;
	border-radius: 20px;
	border: 1px solid transparent;
	font-size: 14px;
	font-weight: 600;
	line-height: 38px;
	transition: background-color 0.25s, border-color 0.25s, color 0.25s;
}

.button.brand {
	background: var(--vp-button-brand-bg);
	color: var(--vp-button-brand-text);
}

.button.brand:hover {
	background: var(--vp-button-brand-hover-bg);
}

.button.alt {
	border-color: var(--vp-button-alt-border);
	background: var(--vp-button-alt-bg);
	color: var(--vp-button-alt-text);
}

.button.alt:hover {
	border-color: var(--vp-button-alt-hover-border);
	background: var(--vp-button-alt-hover-bg);
}

.platforms-title {
	margin-top: 80px;
}

.platforms {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
	gap: 16px;
	margin-top: 32px;
}

.platform {
	display: flex;
	flex-direction: column;
	gap: 4px;
	padding: 20px 24px;
	border-radius: 14px;
	border: 1px solid var(--vp-c-divider);
}

.platform strong {
	font-size: 18px;
}

.platform .detail {
	color: var(--vp-c-brand-1);
	font-size: 13px;
	font-weight: 600;
}

.platform .how {
	color: var(--vp-c-text-2);
	font-size: 14px;
}
</style>
