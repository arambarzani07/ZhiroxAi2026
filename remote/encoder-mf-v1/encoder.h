#pragma once

#include <cstddef>
#include <cstdint>

#ifdef _WIN32
#define ZHIROX_ENCODER_API extern "C" __declspec(dllexport)
#else
#define ZHIROX_ENCODER_API extern "C"
#endif

struct ZhiroxEncoder;

ZHIROX_ENCODER_API long zhirox_encoder_create(
    uint32_t width,
    uint32_t height,
    uint32_t fps,
    uint32_t bitrate,
    ZhiroxEncoder** out_encoder);

ZHIROX_ENCODER_API void zhirox_encoder_destroy(ZhiroxEncoder* encoder);

// Encodes one BGRA desktop frame. The output is the raw H.264 elementary
// stream bytes produced by the Windows Media Foundation encoder.
//
// Returns S_OK when output was produced. If the encoder needs more input,
// out_size is set to zero and S_OK is returned. If output_capacity is too
// small, out_size receives the required byte count and HRESULT_FROM_WIN32(
// ERROR_INSUFFICIENT_BUFFER) is returned.
ZHIROX_ENCODER_API long zhirox_encoder_encode_bgra(
    ZhiroxEncoder* encoder,
    const uint8_t* bgra,
    uint32_t stride,
    int64_t timestamp_100ns,
    int force_keyframe,
    uint8_t* output,
    size_t output_capacity,
    size_t* out_size);
