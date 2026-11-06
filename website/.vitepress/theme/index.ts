import DefaultTheme from 'vitepress/theme';
import type { Theme } from 'vitepress';
import HomeShowcase from './components/HomeShowcase.vue';
import HomeSteps from './components/HomeSteps.vue';
import './style.css';

export default {
	extends: DefaultTheme,
	enhanceApp({ app }) {
		app.component('HomeShowcase', HomeShowcase);
		app.component('HomeSteps', HomeSteps);
	},
} satisfies Theme;
