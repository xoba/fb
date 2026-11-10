#version 330 core
// Tint an icon toward the highlight color while its file is selected.

in vec2 uv;
out vec4 fragColor;

uniform sampler2D icon;
uniform vec3 highlight;
uniform float amount;

void main() {
    vec4 texel = texture(icon, uv);
    vec3 tinted = mix(texel.rgb, highlight, clamp(amount, 0.0, 1.0));
    fragColor = vec4(tinted, texel.a);
}
