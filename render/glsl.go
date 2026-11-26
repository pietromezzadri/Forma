package render

// Faces: a studio-style CAD look — a key light, a fill from the camera, a
// cool sky / warm ground ambient and a little Fresnel rim. PICK writes the
// ID instead.
const faceVert = `#version 410 core
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aNrm;
layout(location = 2) in uint aID;
uniform mat4 uView;
uniform mat4 uProj;
uniform vec4 uClip; // a section: kept where dot(xyz, p) + w >= 0
uniform vec3 uOffset; // moves the drawing (an exploded view)
out vec3 vNrm;
out vec3 vViewPos;
flat out uint vID;
void main() {
	gl_ClipDistance[0] = dot(uClip.xyz, (aPos + uOffset)) + uClip.w;
	vec4 vp = uView * vec4((aPos + uOffset), 1.0);
	vViewPos = vp.xyz;
	vNrm = mat3(uView) * aNrm;
	vID = aID;
	gl_Position = uProj * vp;
}`

const faceFrag = `#version 410 core
in vec3 vNrm;
in vec3 vViewPos;
flat in uint vID;
uniform vec4 uColor;
uniform vec3 uKeyDir;   // view space, toward the light
uniform uint uHover;
uniform vec4 uHoverColor;
uniform int uOrtho;
uniform int uSection;
uniform vec3 uGlow;     // light given off (a lit LED), added past the shading
#ifdef PICK
uniform uint uIDBase;
out uint oID;
#else
out vec4 oColor;
#endif
void main() {
#ifdef PICK
	oID = uIDBase | vID;
#else
	vec3 n = normalize(vNrm);
	vec3 v = uOrtho == 1 ? vec3(0, 0, 1) : normalize(-vViewPos);
	if (!gl_FrontFacing) n = -n;
	vec3 base = uColor.rgb;
	if (uHover != 0u && vID == uHover) base = mix(base, uHoverColor.rgb, uHoverColor.a);
	float key = max(dot(n, normalize(uKeyDir)), 0.0);
	float head = max(dot(n, v), 0.0);
	float up = n.y * 0.5 + 0.5;
	vec3 amb = mix(vec3(0.26, 0.25, 0.24), vec3(0.36, 0.40, 0.46), up);
	vec3 h = normalize(normalize(uKeyDir) + v);
	float spec = pow(max(dot(n, h), 0.0), 48.0) * 0.18;
	float rim = pow(1.0 - head, 3.0) * 0.12;
	vec3 c = base * (amb + 0.48 * key + 0.30 * head) + vec3(spec + rim) + uGlow;
	if (!gl_FrontFacing) {
		if (uSection == 1) {
			// the cut, seen through the section plane: hatched
			float h = mod(gl_FragCoord.x + gl_FragCoord.y, 9.0);
			c = base * 0.62 * (h < 1.6 ? 0.55 : 1.0);
		} else {
			c = vec3(0.75, 0.25, 0.25) * (0.4 + 0.6 * head); // inside-out shows red
		}
	}
	oColor = vec4(pow(clamp(c, 0.0, 1.0), vec3(1.0 / 2.2)), uColor.a);
#endif
}`

// Lines become screen-space quads in the geometry shader, so they have a
// width in pixels on core profiles. aDist (world length along the polyline)
// drives dashes.
const lineVert = `#version 410 core
layout(location = 0) in vec3 aPos;
layout(location = 2) in uint aID;
layout(location = 3) in float aDist;
uniform mat4 uView;
uniform mat4 uProj;
uniform vec4 uClip;
uniform vec3 uOffset;
flat out uint gID;
out float gDist;
void main() {
	gID = aID;
	gDist = aDist;
	gl_ClipDistance[0] = dot(uClip.xyz, (aPos + uOffset)) + uClip.w;
	gl_Position = uProj * uView * vec4((aPos + uOffset), 1.0);
}`

const lineGeom = `#version 410 core
layout(lines) in;
layout(triangle_strip, max_vertices = 4) out;
flat in uint gID[];
in float gDist[];
uniform vec2 uViewport;
uniform float uWidth;
uniform float uDepthBias;
uniform float uDepthNear;
flat out uint vID;
out float vDist;
void main() {
	vec4 a = gl_in[0].gl_Position;
	vec4 b = gl_in[1].gl_Position;
	float da = gDist[0], db = gDist[1];
	float ca = gl_in[0].gl_ClipDistance[0], cb = gl_in[1].gl_ClipDistance[0];
	// clip against the near plane (w > eps)
	float eps = 1e-5;
	if (a.w < eps && b.w < eps) return;
	if (a.w < eps) { float t = (eps - a.w) / (b.w - a.w); a = mix(a, b, t); da = mix(da, db, t); ca = mix(ca, cb, t); }
	if (b.w < eps) { float t = (eps - b.w) / (a.w - b.w); b = mix(b, a, t); db = mix(db, da, t); cb = mix(cb, ca, t); }
	vec2 sa = a.xy / a.w * uViewport * 0.5;
	vec2 sb = b.xy / b.w * uViewport * 0.5;
	vec2 d = sb - sa;
	if (dot(d, d) < 1e-8) d = vec2(1, 0);
	vec2 n = normalize(vec2(-d.y, d.x)) * uWidth * 0.5;
	vec2 e = normalize(d) * uWidth * 0.25;
	vec2 off[4] = vec2[4](-n - e, n - e, -n + e, n + e);
	for (int i = 0; i < 4; i++) {
		vec4 p = i < 2 ? a : b;
		vec2 o = off[i] / (uViewport * 0.5) * p.w;
		gl_Position = vec4(p.xy + o, p.z - uDepthBias * p.w - uDepthNear, p.w);
		vID = gID[0];
		vDist = i < 2 ? da : db;
		gl_ClipDistance[0] = i < 2 ? ca : cb;
		EmitVertex();
	}
	EndPrimitive();
}`

const lineFrag = `#version 410 core
flat in uint vID;
in float vDist;
uniform vec4 uColor;
uniform uint uHover;
uniform vec4 uHoverColor;
uniform float uDash; // dash period in world units, 0 = solid
#ifdef PICK
uniform uint uIDBase;
out uint oID;
#else
out vec4 oColor;
#endif
void main() {
	if (uDash > 0.0 && fract(vDist / uDash) > 0.55) discard;
#ifdef PICK
	oID = uIDBase | vID;
#else
	vec4 c = uColor;
	if (uHover != 0u && vID == uHover) c = uHoverColor;
	oColor = c;
#endif
}`

const pointGeom = `#version 410 core
layout(points) in;
layout(triangle_strip, max_vertices = 4) out;
flat in uint gID[];
in float gDist[];
uniform vec2 uViewport;
uniform float uWidth;
uniform float uDepthBias;
uniform float uDepthNear;
flat out uint vID;
out vec2 vCorner;
void main() {
	vec4 p = gl_in[0].gl_Position;
	if (p.w < 1e-5) return;
	vec2 c[4] = vec2[4](vec2(-1, -1), vec2(1, -1), vec2(-1, 1), vec2(1, 1));
	for (int i = 0; i < 4; i++) {
		vec2 o = c[i] * uWidth * 0.5 / (uViewport * 0.5) * p.w;
		gl_Position = vec4(p.xy + o, p.z - uDepthBias * p.w - uDepthNear, p.w);
		vID = gID[0];
		vCorner = c[i];
		gl_ClipDistance[0] = gl_in[0].gl_ClipDistance[0];
		EmitVertex();
	}
	EndPrimitive();
}`

const pointFrag = `#version 410 core
flat in uint vID;
in vec2 vCorner;
uniform vec4 uColor;
uniform uint uHover;
uniform vec4 uHoverColor;
#ifdef PICK
uniform uint uIDBase;
out uint oID;
#else
out vec4 oColor;
#endif
void main() {
	float r = length(vCorner);
#ifdef PICK
	if (r > 1.0) discard;
	oID = uIDBase | vID;
#else
	if (r > 1.0) discard;
	vec4 c = uColor;
	if (uHover != 0u && vID == uHover) c = uHoverColor;
	float edge = smoothstep(1.0, 0.75, r);
	oColor = vec4(mix(vec3(0.05), c.rgb, smoothstep(0.85, 0.55, r)), c.a * edge);
#endif
}`

// The infinite grid on a plane (uPlane: origin, X and Y axes), drawn from a
// full-screen triangle by intersecting each pixel's ray with it.
const gridVert = `#version 410 core
out vec2 vNDC;
void main() {
	vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2) * 2.0 - 1.0;
	vNDC = p;
	gl_Position = vec4(p, 0.0, 1.0);
}`

const gridFrag = `#version 410 core
in vec2 vNDC;
uniform mat4 uInvViewProj;
uniform mat4 uViewProj;
uniform vec3 uOrigin;
uniform vec3 uAxisX;
uniform vec3 uAxisY;
uniform float uStep;     // minor spacing (mm)
uniform float uFade;     // distance where the grid has faded
uniform vec3 uEye;
uniform vec4 uMinor;
uniform vec4 uMajor;
uniform vec4 uAxisXColor;
uniform vec4 uAxisYColor;
out vec4 oColor;

float gridLine(vec2 c, float step) {
	vec2 g = c / step;
	vec2 w = fwidth(g);
	vec2 a = abs(fract(g - 0.5) - 0.5) / max(w, 1e-6);
	return 1.0 - min(min(a.x, a.y), 1.0);
}

void main() {
	vec4 n = uInvViewProj * vec4(vNDC, -1.0, 1.0);
	vec4 f = uInvViewProj * vec4(vNDC, 1.0, 1.0);
	vec3 ro = n.xyz / n.w;
	vec3 rd = f.xyz / f.w - ro;
	vec3 pn = normalize(cross(uAxisX, uAxisY));
	float den = dot(rd, pn);
	if (abs(den) < 1e-9) discard;
	float t = dot(uOrigin - ro, pn) / den;
	if (t < 0.0 || t > 1.0) discard;
	vec3 p = ro + rd * t;
	vec2 c = vec2(dot(p - uOrigin, uAxisX), dot(p - uOrigin, uAxisY));

	float minor = gridLine(c, uStep);
	float major = gridLine(c, uStep * 10.0);
	vec4 col = vec4(0.0);
	col = mix(col, uMinor, minor * uMinor.a);
	col = mix(col, uMajor, major);
	vec2 w = fwidth(c);
	float ax = 1.0 - min(abs(c.y) / max(w.y, 1e-6) / 1.5, 1.0); // the X axis: y = 0
	float ay = 1.0 - min(abs(c.x) / max(w.x, 1e-6) / 1.5, 1.0);
	col = mix(col, uAxisXColor, ax);
	col = mix(col, uAxisYColor, ay);

	float dist = length(p - uEye);
	float fade = clamp(1.0 - dist / uFade, 0.0, 1.0);
	// fade lines seen edge-on
	float grazing = clamp(abs(dot(normalize(rd), pn)) * 4.0, 0.0, 1.0);
	col.a *= fade * fade * mix(0.25, 1.0, grazing);
	if (col.a < 0.003) discard;
	oColor = col;
	vec4 clip = uViewProj * vec4(p, 1.0);
	gl_FragDepth = (clip.z / clip.w) * 0.5 + 0.5;
}`

// Flat-coloured triangles (sketch regions, previews), alpha blended.
const flatVert = `#version 410 core
layout(location = 0) in vec3 aPos;
layout(location = 2) in uint aID;
uniform mat4 uView;
uniform mat4 uProj;
uniform vec4 uClip;
uniform vec3 uOffset; // moves the drawing (an exploded view)
flat out uint vID;
void main() {
	vID = aID;
	gl_ClipDistance[0] = dot(uClip.xyz, (aPos + uOffset)) + uClip.w;
	gl_Position = uProj * uView * vec4((aPos + uOffset), 1.0);
}`

const flatFrag = `#version 410 core
flat in uint vID;
uniform vec4 uColor;
uniform uint uHover;
uniform vec4 uHoverColor;
#ifdef PICK
uniform uint uIDBase;
out uint oID;
#else
out vec4 oColor;
#endif
void main() {
#ifdef PICK
	oID = uIDBase | vID;
#else
	vec4 c = uColor;
	if (uHover != 0u && vID == uHover) c = uHoverColor;
	if (c.a <= 0.0) discard;
	oColor = c;
#endif
}`
