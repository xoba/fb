// Tint an icon toward the highlight color while its file is selected.
#include <metal_stdlib>
using namespace metal;

struct Selection {
    float3 highlight;
    float amount;
};

struct VertexOut {
    float4 position [[position]];
    float2 uv;
};

fragment float4 tint_icon(VertexOut in [[stage_in]],
                          texture2d<float> icon [[texture(0)]],
                          constant Selection &selection [[buffer(0)]])
{
    constexpr sampler bilinear(filter::linear);
    float4 texel = icon.sample(bilinear, in.uv);
    float3 tinted = mix(texel.rgb, selection.highlight, saturate(selection.amount));
    return float4(tinted, texel.a);
}
