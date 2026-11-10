// Tint an icon toward the highlight color while its file is selected.
Texture2D Icon : register(t0);
SamplerState LinearSampler : register(s0);

cbuffer Selection : register(b0)
{
    float3 Highlight;
    float Amount;
};

float4 main(float4 position : SV_Position, float2 uv : TEXCOORD0) : SV_Target
{
    float4 texel = Icon.Sample(LinearSampler, uv);
    float3 tinted = lerp(texel.rgb, Highlight, saturate(Amount));
    return float4(tinted, texel.a);
}
