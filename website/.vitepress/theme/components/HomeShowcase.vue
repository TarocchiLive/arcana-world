<script setup lang="ts">
import { computed, ref } from 'vue';
import { withBase } from 'vitepress';

// 首页界面预览：按标签切换截图，图片来自仓库 assets/。
const t = {
	title: '界面预览',
	lead: '提供终端界面和网页面板，弹幕可浮层显示在桌面上。',
	tabs: [
		{ label: '终端界面', image: '/images/arcana-world-01.webp', caption: '数字键切换页面，开播和停播在「直播」页操作。' },
		{ label: '弹幕与管理', image: '/images/arcana-world-03.webp', caption: '点击观众昵称可禁言、拉黑或设为房管。' },
		{ label: '网页面板', image: '/images/arcana-world-07.webp', caption: '鼠标操作；关闭浏览器不会停播。' },
		{ label: '弹幕浮层', image: '/images/arcana-world-05.webp', caption: '半透明、鼠标可穿透，提供简洁和精美两种样式。' },
	],
};

const active = ref(0);
const current = computed(() => t.tabs[active.value]);
</script>

<template>
	<section class="home-showcase">
		<h2 class="section-title">{{ t.title }}</h2>
		<p class="section-lead">{{ t.lead }}</p>
		<div class="tabs" role="tablist">
			<button
				v-for="(tab, i) in t.tabs"
				:key="tab.label"
				type="button"
				role="tab"
				:aria-selected="active === i"
				:class="{ active: active === i }"
				@click="active = i"
			>
				{{ tab.label }}
			</button>
		</div>
		<figure class="frame">
			<img :src="withBase(current.image)" :alt="current.label" loading="lazy" />
		</figure>
		<p class="caption">{{ current.caption }}</p>
	</section>
</template>

<style scoped>
.home-showcase {
	margin: 0 auto;
	padding: 72px 24px 0;
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
	max-width: 600px;
	text-align: center;
	color: var(--vp-c-text-2);
	line-height: 1.7;
}

.tabs {
	display: flex;
	flex-wrap: wrap;
	justify-content: center;
	gap: 8px;
	margin-top: 28px;
}

.tabs button {
	padding: 0 16px;
	border-radius: 18px;
	border: 1px solid var(--vp-c-divider);
	color: var(--vp-c-text-2);
	font-size: 14px;
	font-weight: 500;
	line-height: 34px;
	transition: color 0.2s, border-color 0.2s, background-color 0.2s;
}

.tabs button:hover {
	color: var(--vp-c-text-1);
	border-color: var(--vp-c-brand-2);
}

.tabs button.active {
	border-color: var(--vp-c-brand-1);
	background: var(--vp-c-brand-soft);
	color: var(--vp-c-brand-1);
}

.frame {
	display: flex;
	align-items: center;
	justify-content: center;
	margin: 24px auto 0;
	padding: 16px;
	max-width: 960px;
	aspect-ratio: 16 / 10;
	border-radius: 16px;
	border: 1px solid var(--vp-c-divider);
	background:
		radial-gradient(circle at 20% 0%, var(--vp-c-brand-soft), transparent 60%),
		var(--vp-c-bg-soft);
}

.frame img {
	max-width: 100%;
	max-height: 100%;
	border-radius: 10px;
	box-shadow: 0 12px 32px rgba(0, 0, 0, 0.12);
}

.caption {
	margin: 16px auto 0;
	max-width: 640px;
	text-align: center;
	color: var(--vp-c-text-2);
	font-size: 14px;
	line-height: 1.7;
}
</style>
