package design

import "testing"

// 2.2.2: uploaded photos get smaller copies for smaller screens, in image layers and in post content.
// 2.3.0 (Qraft feedback 39): the first images of a page load at once, and a fixed-width layer says its width
// in sizes, so a small logo takes a small copy.
func TestRenderResponsiveImages(t *testing.T) {
	const doc = `{"id":"root","type":"frame","children":[
	 {"id":"logo","type":"image","width":{"mode":"fixed","value":30},"responsive":{"mobile":{"width":{"mode":"fixed","value":24}}},"props":{"src":"/uploads/logo.png","alt":"logo"}},
	 {"id":"i","type":"image","props":{"src":"{{post.image}}","alt":"cover"}},
	 {"id":"x","type":"image","props":{"src":"https://cdn.test/a.jpg"}},
	 {"id":"w","type":"image","width":{"mode":"fixed","value":300},"responsive":{"mobile":{"width":{"mode":"fill"}}},"props":{"src":"/uploads/w.jpg"}},
	 {"id":"body","type":"postContent"}
	]}`
	post := PostCtx{"image": "/uploads/2026/11/cover.jpg", "content": `<p>t</p><img src="/uploads/2026/11/in.png" alt="in">`}
	out := renderDoc(t, doc, Context{Post: post}, nil)
	must(t, out.HTML,
		`<img class="n-logo" src="/uploads/logo.png" srcset="/uploads/logo.png?w=96 96w, /uploads/logo.png?w=160 160w, `,
		`/uploads/logo.png?w=1600 1600w" sizes="30px" alt="logo">`,
		`<img class="n-i" src="/uploads/2026/11/cover.jpg" srcset="/uploads/2026/11/cover.jpg?w=96 96w, /uploads/2026/11/cover.jpg?w=160 160w, /uploads/2026/11/cover.jpg?w=320 320w, /uploads/2026/11/cover.jpg?w=480 480w, /uploads/2026/11/cover.jpg?w=768 768w, /uploads/2026/11/cover.jpg?w=1200 1200w, /uploads/2026/11/cover.jpg?w=1600 1600w" sizes="100vw" alt="cover">`,
		`<img class="n-x" src="https://cdn.test/a.jpg" alt="" loading="lazy">`,
		`/uploads/w.jpg?w=1600 1600w" sizes="auto, 100vw" alt="" loading="lazy">`,
		`<img srcset="/uploads/2026/11/in.png?w=96 96w`, `sizes="auto, 100vw" loading="lazy" src="/uploads/2026/11/in.png" alt="in">`)
}
