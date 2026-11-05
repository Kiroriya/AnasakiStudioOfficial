package design

import "testing"

// 2.2.2: uploaded photos get smaller copies for smaller screens, in image layers and in post content.
func TestRenderResponsiveImages(t *testing.T) {
	const doc = `{"id":"root","type":"frame","children":[
	 {"id":"i","type":"image","props":{"src":"{{post.image}}","alt":"cover"}},
	 {"id":"x","type":"image","props":{"src":"https://cdn.test/a.jpg"}},
	 {"id":"body","type":"postContent"}
	]}`
	post := PostCtx{"image": "/uploads/2026/11/cover.jpg", "content": `<p>t</p><img src="/uploads/2026/11/in.png" alt="in">`}
	out := renderDoc(t, doc, Context{Post: post}, nil)
	must(t, out.HTML,
		`<img class="n-i" src="/uploads/2026/11/cover.jpg" srcset="/uploads/2026/11/cover.jpg?w=480 480w, /uploads/2026/11/cover.jpg?w=768 768w, /uploads/2026/11/cover.jpg?w=1200 1200w, /uploads/2026/11/cover.jpg?w=1600 1600w" sizes="auto, 100vw" alt="cover" loading="lazy">`,
		`<img class="n-x" src="https://cdn.test/a.jpg" alt="" loading="lazy">`,
		`<img srcset="/uploads/2026/11/in.png?w=480 480w`, `sizes="auto, 100vw" loading="lazy" src="/uploads/2026/11/in.png" alt="in">`)
}
