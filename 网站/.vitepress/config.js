import {defineConfig} from 'vitepress';

function replace_link(md) {
    md.core.ruler.after('inline', 'replace-link', function (state) {
        for (const block of state.tokens) {
            if (block.type === 'inline' && block.children) {
                for (const token of block.children) {
                    const href = token.attrGet('href');
                    if (href && href.indexOf('说明书.md') >= 0) {
                        // token.attrJoin('style', 'color:red;');
                        token.attrSet('href', href.replace('说明书.md', 'index.md'));
                    }
                }
            }
        }
        return true;
    });
}

export default defineConfig({
    title: 'go2rtc',
    lang: 'zh-CN',
    description: '功能丰富的摄像头流媒体应用',
    head: [
        // Telegram 卡片第一行绿色粗体文字，默认从主机名检测
        ['meta', { property: 'og:site_name', content: 'go2rtc.org' }],
        // Telegram 卡片第二行黑色粗体文字，默认从站点描述检测
        ['meta', { property: 'og:title', content: 'go2rtc - 功能丰富的摄像头流媒体应用' }],
        // Telegram 卡片第三行文字，默认从站点描述检测
        ['meta', { property: 'og:description', content: '支持 alsa, doorbird, dvrip, eseecloud, ffmpeg, gopro, hass, hls, homekit, mjpeg, mp4, mpegts, nest, onvif, ring, roborock, rtmp, rtsp, tapo, vigi, tuya, v4l2, webrtc, wyze, xiaomi.' }],
        ['meta', { property: 'og:url', content: 'https://go2rtc.org/' }],
        ['meta', { property: 'og:image', content: 'https://go2rtc.org/images/logo.png' }],
        // Telegram 需要将大图放在卡片底部
        ['meta', { property: 'twitter:card', content: 'summary_large_image' }],
    ],
    sitemap: {hostname: 'https://go2rtc.org'},

    themeConfig: {
        nav: [
            {text: '首页', link: '/'},
        ],
        sidebar: [
            {
                items: [
                    {text: '安装', link: '/#installation'},
                    {text: '配置', link: '/#configuration'},
                    {text: '安全', link: '/#security'},
                ],
            },
            {
                text: '功能',
                items: [
                    {text: '流输入', link: '/#streaming-input'},
                    {text: '流输出', link: '/#streaming-output'},
                    {text: '接收推流', link: '/#streaming-ingest'},
                    {text: '双向对讲', link: '/#two-way-audio'},
                    {text: '向摄像头播放音频', link: '/#stream-to-camera'},
                    {text: '推送流', link: '/#publish-stream'},
                    {text: '预加载流', link: '/#preload-stream'},
                    {text: '流统计', link: '/#streaming-stats'},
                ],
                collapsed: false,
            },
            {
                text: '编码格式',
                items: [
                    {text: '编码过滤', link: '/#codecs-filters'},
                    {text: '编码兼容性', link: '/#codecs-madness'},
                    {text: '内置转码', link: '/#built-in-transcoding'},
                    {text: '编码协商', link: '/#codecs-negotiation'},
                ],
                collapsed: true,
            },
            {
                text: '其他',
                items: [
                    {text: '使用 go2rtc 的项目', link: '/#projects-using-go2rtc'},
                    {text: '摄像头使用经验', link: '/#camera-experience'},
                    {text: '使用技巧', link: '/#tips'},
                ],
                collapsed: true,
            },
            {
                text: '核心模块',
                items: [
                    {text: 'app', link: '/internal/app/'},
                    {text: 'api', link: '/internal/api/'},
                    {text: 'streams', link: '/internal/streams/'},
                ],
                collapsed: false,
            },
            {
                text: '主要模块',
                items: [
                    {text: 'http', link: '/internal/http/'},
                    {text: 'mjpeg', link: '/internal/mjpeg/'},
                    {text: 'mp4', link: '/internal/mp4/'},
                    {text: 'rtsp', link: '/internal/rtsp/'},
                    {text: 'webrtc', link: '/internal/webrtc/'},
                ],
                collapsed: false,
            },
            {
                text: '其他模块',
                items: [
                    {text: 'hls', link: '/internal/hls/'},
                    {text: 'homekit', link: '/internal/homekit/'},
                    {text: 'onvif', link: '/internal/onvif/'},
                    {text: 'rtmp', link: '/internal/rtmp/'},
                    {text: 'webtorrent', link: '/internal/webtorrent/'},
                    {text: 'wyoming', link: '/internal/wyoming/'},
                ],
                collapsed: false,
            },
            {
                text: '脚本源',
                items: [
                    {text: 'echo', link: '/internal/echo/'},
                    {text: 'exec', link: '/internal/exec/'},
                    {text: 'expr', link: '/internal/expr/'},
                    {text: 'ffmpeg', link: '/internal/ffmpeg/'},
                ],
                collapsed: false,
            },
            {
                text: '其他源',
                items: [
                    {text: 'alsa', link: '/internal/alsa/'},
                    {text: 'bubble', link: '/internal/bubble/'},
                    {text: 'doorbird', link: '/internal/doorbird/'},
                    {text: 'dvrip', link: '/internal/dvrip/'},
                    {text: 'eseecloud', link: '/internal/eseecloud/'},
                    {text: 'flussonic', link: '/internal/flussonic/'},
                    {text: 'gopro', link: '/internal/gopro/'},
                    {text: 'hass', link: '/internal/hass/'},
                    {text: 'isapi', link: '/internal/isapi/'},
                    {text: 'ivideon', link: '/internal/ivideon/'},
                    {text: 'kasa', link: '/internal/kasa/'},
                    {text: 'mpeg', link: '/internal/mpeg/'},
                    {text: 'multitrans', link: '/internal/multitrans/'},
                    {text: 'nest', link: '/internal/nest/'},
                    {text: 'ring', link: '/internal/ring/'},
                    {text: 'roborock', link: '/internal/roborock/'},
                    {text: 'tapo', link: '/internal/tapo/'},
                    {text: 'tuya', link: '/internal/tuya/'},
                    {text: 'v4l2', link: '/internal/v4l2/'},
                    {text: 'wyze', link: '/internal/wyze/'},
                    {text: 'xiaomi', link: '/internal/xiaomi/'},
                    {text: 'yandex', link: '/internal/yandex/'},
                ],
                collapsed: false,
            },
            {
                text: '辅助模块',
                items: [
                    {text: 'debug', link: '/internal/debug/'},
                    {text: 'ngrok', link: '/internal/ngrok/'},
                    {text: 'pinggy', link: '/internal/pinggy/'},
                    {text: 'srtp', link: '/internal/srtp/'},
                ],
                collapsed: false,
            },

        ],
        socialLinks: [
            {icon: 'github', link: 'https://github.com/AlexxIT/go2rtc'}
        ],
        outline: [2, 3],
        search: {provider: 'local', options: {translations: {button: {buttonText: '搜索文档', buttonAriaLabel: '搜索文档'}, modal: {displayDetails: '显示详细内容', resetButtonTitle: '清空搜索', backButtonTitle: '返回', noResultsText: '未找到相关内容', footer: {selectText: '选择', navigateText: '切换', closeText: '关闭'}}}}},
        outlineTitle: '本页目录',
        docFooter: {prev: '上一篇', next: '下一篇'},
        sidebarMenuLabel: '菜单',
        returnToTopLabel: '回到顶部',
        darkModeSwitchLabel: '外观',
        lightModeSwitchTitle: '切换到浅色模式',
        darkModeSwitchTitle: '切换到深色模式',
    },

    rewrites(id) {
        // 修改文件名称
        return id.replace('说明书.md', 'index.md');
    },

    markdown: {
        config: (md) => {
            // 修改 Markdown 链接
            md.use(replace_link);
        }
    },

    srcDir: '..',
    srcExclude: ['examples/', 'pkg/', 'README.md'],

    // cleanUrls: true,
    ignoreDeadLinks: true,
});
