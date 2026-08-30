// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// 文档站面向 scx-rg 用户（不是开发者）：只讲安装、用法、键位、配置与排障；
// 架构与代码导航见仓库根 README 与 .wiki/。
export default defineConfig({
  // 本地 dev/preview 直接跑在根路径;部署到 GitHub Pages 项目站点时由
  // CI 注入 DOCS_BASE=/scx-rg(.github/workflows/docs.yml),两端 URL 都正确。
  site: 'https://shawricx.github.io',
  base: process.env.DOCS_BASE ?? '/',
  integrations: [
    starlight({
      title: 'scx-rg',
      description: '终端里的实时搜索与日志检索工具',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/shawricx/scx-rg' },
      ],
      locales: {
        root: { label: '简体中文', lang: 'zh-CN' },
      },
      logo: {
        src: './src/assets/logo.svg',
      },
      sidebar: [
        {
          label: '指南',
          items: [
            { label: '开始使用', link: '/guides/getting-started/' },
            { label: '搜索与模式', link: '/guides/search/' },
            { label: '日志检索（Docker / K8s / 跟随）', link: '/guides/logs/' },
            { label: '结果筛选（Ctrl+T）', link: '/guides/filtering/' },
            { label: '键位参考', link: '/guides/keybindings/' },
            { label: '集成与组合', link: '/guides/integrations/' },
          ],
        },
        {
          label: '参考',
          items: [
            { label: '配置文件', link: '/reference/configuration/' },
            { label: '命令行参数', link: '/reference/cli/' },
            { label: '常见问题', link: '/reference/faq/' },
          ],
        },
      ],
    }),
  ],
});
