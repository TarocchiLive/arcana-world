---
layout: home
title: Arcana World
titleTemplate: bilibili 直播姬 + 弹幕姬
markdownStyles: false

hero:
  name: Arcana World
  text: bilibili 直播姬 + 弹幕姬
  tagline: 扫码登录 B 站，连接 OBS 后一键开播；集成弹幕收发、桌面弹幕浮层和语音播报。
  image:
    src: /images/arcana-world.webp
    alt: Arcana World 世界塔罗牌插画
  actions:
    - theme: brand
      text: 快速上手
      link: /guide/start-web
    - theme: alt
      text: 下载
      link: https://github.com/TarocchiLive/arcana-world/releases/latest
    - theme: alt
      text: 了解更多
      link: /guide/introduction

features:
  - icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polygon points="10 8 16 12 10 16 10 8"/></svg>
    title: 一键开播
    details: 开播时自动将推流地址写入 OBS 并开始推流，无需手动复制推流码。
    link: /guide/start-web
    linkText: 快速上手
  - icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
    title: 弹幕收发与管理
    details: 查看、发送弹幕和浏览历史；点击观众昵称即可禁言、拉黑或设为房管。
    link: /features/chat
    linkText: 弹幕与观众管理
  - icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>
    title: 桌面弹幕浮层
    details: 半透明显示在桌面最上层，鼠标点击可穿透。
    link: /features/overlay
    linkText: 弹幕浮层
  - icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14M15.54 8.46a5 5 0 0 1 0 7.07"/></svg>
    title: 语音播报
    details: 语音播报弹幕、礼物和醒目留言。支持免费的 Edge 语音、千问、豆包和离线本地语音。
    link: /features/tts
    linkText: 语音播报
  - icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
    title: 终端或浏览器
    details: 提供键盘操作的终端界面和浏览器中的网页面板，两者共用账号与设置。
    link: /guide/terminal
    linkText: 终端入门
  - icon: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    title: 开放的弹幕数据
    details: 通过本机 WebSocket 广播实时事件，可接入 OBS 浏览器源或自定义脚本。
    link: /features/websocket
    linkText: WebSocket 广播
---

<HomeShowcase />

<HomeSteps />
