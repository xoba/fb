// Tint each pixel of a texture toward a uniform color.
struct Params {
    tint: vec4<f32>,
    strength: f32,
}

@group(0) @binding(0) var<uniform> params: Params;
@group(0) @binding(1) var source: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

struct VertexOut {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

@fragment
fn fs_main(vout: VertexOut) -> @location(0) vec4<f32> {
    let color = textureSample(source, samp, vout.uv);
    return mix(color, params.tint, params.strength);
}
