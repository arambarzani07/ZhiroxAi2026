#include "encoder.h"

#ifdef _WIN32

#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mftransform.h>
#include <mferror.h>
#include <codecapi.h>
#include <wrl/client.h>
#include <algorithm>
#include <cstdint>
#include <new>
#include <vector>

using Microsoft::WRL::ComPtr;

namespace {

uint8_t clamp8(int v) {
    if (v < 0) return 0;
    if (v > 255) return 255;
    return static_cast<uint8_t>(v);
}

void bgra_to_nv12(
    const uint8_t* bgra,
    uint32_t stride,
    uint32_t width,
    uint32_t height,
    std::vector<uint8_t>& nv12) {
    const size_t y_size = static_cast<size_t>(width) * height;
    nv12.resize(y_size + y_size / 2);
    uint8_t* y_plane = nv12.data();
    uint8_t* uv_plane = nv12.data() + y_size;

    for (uint32_t y = 0; y < height; ++y) {
        const uint8_t* row = bgra + static_cast<size_t>(y) * stride;
        uint8_t* y_out = y_plane + static_cast<size_t>(y) * width;
        for (uint32_t x = 0; x < width; ++x) {
            const uint8_t b = row[x * 4 + 0];
            const uint8_t g = row[x * 4 + 1];
            const uint8_t r = row[x * 4 + 2];
            y_out[x] = clamp8(((66 * r + 129 * g + 25 * b + 128) >> 8) + 16);
        }
    }

    for (uint32_t y = 0; y < height; y += 2) {
        uint8_t* uv_out = uv_plane + static_cast<size_t>(y / 2) * width;
        for (uint32_t x = 0; x < width; x += 2) {
            int u_sum = 0;
            int v_sum = 0;
            for (uint32_t dy = 0; dy < 2; ++dy) {
                const uint8_t* row = bgra + static_cast<size_t>(y + dy) * stride;
                for (uint32_t dx = 0; dx < 2; ++dx) {
                    const uint8_t b = row[(x + dx) * 4 + 0];
                    const uint8_t g = row[(x + dx) * 4 + 1];
                    const uint8_t r = row[(x + dx) * 4 + 2];
                    u_sum += ((-38 * r - 74 * g + 112 * b + 128) >> 8) + 128;
                    v_sum += ((112 * r - 94 * g - 18 * b + 128) >> 8) + 128;
                }
            }
            uv_out[x + 0] = clamp8(u_sum / 4);
            uv_out[x + 1] = clamp8(v_sum / 4);
        }
    }
}

HRESULT create_h264_transform(ComPtr<IMFTransform>& transform) {
    MFT_REGISTER_TYPE_INFO output_info{};
    output_info.guidMajorType = MFMediaType_Video;
    output_info.guidSubtype = MFVideoFormat_H264;

    IMFActivate** activations = nullptr;
    UINT32 count = 0;
    const UINT32 flags =
        MFT_ENUM_FLAG_SYNCMFT |
        MFT_ENUM_FLAG_ASYNCMFT |
        MFT_ENUM_FLAG_HARDWARE |
        MFT_ENUM_FLAG_LOCALMFT |
        MFT_ENUM_FLAG_TRANSCODE_ONLY |
        MFT_ENUM_FLAG_SORTANDFILTER;

    HRESULT hr = MFTEnumEx(
        MFT_CATEGORY_VIDEO_ENCODER,
        flags,
        nullptr,
        &output_info,
        &activations,
        &count);
    if (FAILED(hr)) return hr;
    if (count == 0) {
        CoTaskMemFree(activations);
        return MF_E_TOPO_CODEC_NOT_FOUND;
    }

    HRESULT activation_hr = E_FAIL;
    for (UINT32 i = 0; i < count; ++i) {
        if (activations[i] == nullptr) continue;
        ComPtr<IMFTransform> candidate;
        HRESULT candidate_hr = activations[i]->ActivateObject(IID_PPV_ARGS(&candidate));
        if (SUCCEEDED(candidate_hr) && candidate) {
            transform = candidate;
            activation_hr = S_OK;
            break;
        }
        activation_hr = candidate_hr;
    }

    for (UINT32 i = 0; i < count; ++i) {
        if (activations[i]) activations[i]->Release();
    }
    CoTaskMemFree(activations);
    return activation_hr;
}

HRESULT set_codec_bool(ICodecAPI* codec, const GUID& key, bool value) {
    if (!codec) return E_NOINTERFACE;
    VARIANT v;
    VariantInit(&v);
    v.vt = VT_BOOL;
    v.boolVal = value ? VARIANT_TRUE : VARIANT_FALSE;
    const HRESULT hr = codec->SetValue(&key, &v);
    VariantClear(&v);
    return hr;
}

} // namespace

struct ZhiroxEncoder {
    uint32_t width = 0;
    uint32_t height = 0;
    uint32_t fps = 0;
    uint32_t bitrate = 0;
    LONGLONG frame_duration = 0;
    bool co_uninitialize = false;
    bool mf_started = false;
    ComPtr<IMFTransform> transform;
    ComPtr<ICodecAPI> codec;
    MFT_OUTPUT_STREAM_INFO output_info{};
    std::vector<uint8_t> nv12;

    HRESULT initialize(uint32_t w, uint32_t h, uint32_t frame_rate, uint32_t bits_per_second) {
        if (w < 160 || h < 120 || w > 3840 || h > 2160 || (w & 1) != 0 || (h & 1) != 0) {
            return E_INVALIDARG;
        }
        if (frame_rate < 1 || frame_rate > 60 || bits_per_second < 100000 || bits_per_second > 50000000) {
            return E_INVALIDARG;
        }
        width = w;
        height = h;
        fps = frame_rate;
        bitrate = bits_per_second;
        frame_duration = 10000000LL / static_cast<LONGLONG>(fps);

        HRESULT hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
        if (SUCCEEDED(hr)) {
            co_uninitialize = true;
        } else if (hr != RPC_E_CHANGED_MODE) {
            return hr;
        }

        hr = MFStartup(MF_VERSION, MFSTARTUP_FULL);
        if (FAILED(hr)) return hr;
        mf_started = true;

        hr = create_h264_transform(transform);
        if (FAILED(hr) || !transform) return FAILED(hr) ? hr : E_FAIL;

        ComPtr<IMFAttributes> transform_attributes;
        if (SUCCEEDED(transform->GetAttributes(&transform_attributes)) && transform_attributes) {
            UINT32 async_mft = 0;
            if (SUCCEEDED(transform_attributes->GetUINT32(MF_TRANSFORM_ASYNC, &async_mft)) && async_mft) {
                (void)transform_attributes->SetUINT32(MF_TRANSFORM_ASYNC_UNLOCK, TRUE);
            }
            (void)transform_attributes->SetUINT32(MF_LOW_LATENCY, TRUE);
        }

        (void)transform.As(&codec);
        if (codec) {
            (void)set_codec_bool(codec.Get(), CODECAPI_AVLowLatencyMode, true);
            VARIANT no_b;
            VariantInit(&no_b);
            no_b.vt = VT_UI4;
            no_b.ulVal = 0;
            (void)codec->SetValue(&CODECAPI_AVEncMPVDefaultBPictureCount, &no_b);
            VariantClear(&no_b);
        }

        ComPtr<IMFMediaType> output_type;
        hr = MFCreateMediaType(&output_type);
        if (FAILED(hr)) return hr;
        if (FAILED(hr = output_type->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video))) return hr;
        if (FAILED(hr = output_type->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_H264))) return hr;
        if (FAILED(hr = output_type->SetUINT32(MF_MT_AVG_BITRATE, bitrate))) return hr;
        if (FAILED(hr = MFSetAttributeSize(output_type.Get(), MF_MT_FRAME_SIZE, width, height))) return hr;
        if (FAILED(hr = MFSetAttributeRatio(output_type.Get(), MF_MT_FRAME_RATE, fps, 1))) return hr;
        if (FAILED(hr = MFSetAttributeRatio(output_type.Get(), MF_MT_PIXEL_ASPECT_RATIO, 1, 1))) return hr;
        if (FAILED(hr = output_type->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive))) return hr;
        if (FAILED(hr = output_type->SetUINT32(MF_MT_MPEG2_PROFILE, eAVEncH264VProfile_Base))) return hr;
        if (FAILED(hr = transform->SetOutputType(0, output_type.Get(), 0))) return hr;

        ComPtr<IMFMediaType> input_type;
        hr = MFCreateMediaType(&input_type);
        if (FAILED(hr)) return hr;
        if (FAILED(hr = input_type->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video))) return hr;
        if (FAILED(hr = input_type->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_NV12))) return hr;
        if (FAILED(hr = MFSetAttributeSize(input_type.Get(), MF_MT_FRAME_SIZE, width, height))) return hr;
        if (FAILED(hr = MFSetAttributeRatio(input_type.Get(), MF_MT_FRAME_RATE, fps, 1))) return hr;
        if (FAILED(hr = MFSetAttributeRatio(input_type.Get(), MF_MT_PIXEL_ASPECT_RATIO, 1, 1))) return hr;
        if (FAILED(hr = input_type->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive))) return hr;
        if (FAILED(hr = transform->SetInputType(0, input_type.Get(), 0))) return hr;

        hr = transform->GetOutputStreamInfo(0, &output_info);
        if (FAILED(hr)) return hr;

        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0))) return hr;
        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0))) return hr;
        if (FAILED(hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0))) return hr;
        return S_OK;
    }

    HRESULT collect_output(std::vector<uint8_t>& encoded) {
        for (int iteration = 0; iteration < 16; ++iteration) {
            MFT_OUTPUT_DATA_BUFFER out{};
            out.dwStreamID = 0;
            DWORD status = 0;
            ComPtr<IMFSample> supplied_sample;

            if ((output_info.dwFlags & MFT_OUTPUT_STREAM_PROVIDES_SAMPLES) == 0) {
                HRESULT hr = MFCreateSample(&supplied_sample);
                if (FAILED(hr)) return hr;
                ComPtr<IMFMediaBuffer> buffer;
                const DWORD fallback = static_cast<DWORD>(std::min<uint64_t>(
                    static_cast<uint64_t>(width) * height * 2ULL,
                    16ULL * 1024ULL * 1024ULL));
                const DWORD capacity = std::max<DWORD>(output_info.cbSize, std::max<DWORD>(fallback, 64 * 1024));
                hr = MFCreateMemoryBuffer(capacity, &buffer);
                if (FAILED(hr)) return hr;
                if (FAILED(hr = supplied_sample->AddBuffer(buffer.Get()))) return hr;
                out.pSample = supplied_sample.Get();
            }

            HRESULT hr = transform->ProcessOutput(0, 1, &out, &status);
            if (out.pEvents) {
                out.pEvents->Release();
                out.pEvents = nullptr;
            }
            if (hr == MF_E_TRANSFORM_NEED_MORE_INPUT) return S_OK;
            if (hr == MF_E_TRANSFORM_STREAM_CHANGE) return hr;
            if (FAILED(hr)) return hr;

            ComPtr<IMFSample> produced;
            if (out.pSample) {
                if (out.pSample == supplied_sample.Get()) {
                    produced = supplied_sample;
                } else {
                    produced.Attach(out.pSample);
                }
            }
            if (!produced) continue;

            ComPtr<IMFMediaBuffer> contiguous;
            if (FAILED(hr = produced->ConvertToContiguousBuffer(&contiguous))) return hr;
            BYTE* data = nullptr;
            DWORD max_len = 0;
            DWORD current_len = 0;
            if (FAILED(hr = contiguous->Lock(&data, &max_len, &current_len))) return hr;
            if (data && current_len) {
                encoded.insert(encoded.end(), data, data + current_len);
            }
            contiguous->Unlock();
        }
        return S_OK;
    }

    HRESULT encode(
        const uint8_t* bgra,
        uint32_t stride,
        LONGLONG timestamp,
        bool force_keyframe,
        std::vector<uint8_t>& encoded) {
        if (!bgra || stride < width * 4) return E_INVALIDARG;
        bgra_to_nv12(bgra, stride, width, height, nv12);

        if (force_keyframe && codec) {
            (void)set_codec_bool(codec.Get(), CODECAPI_AVEncVideoForceKeyFrame, true);
        }

        ComPtr<IMFMediaBuffer> buffer;
        HRESULT hr = MFCreateMemoryBuffer(static_cast<DWORD>(nv12.size()), &buffer);
        if (FAILED(hr)) return hr;
        BYTE* destination = nullptr;
        DWORD max_len = 0;
        DWORD current_len = 0;
        if (FAILED(hr = buffer->Lock(&destination, &max_len, &current_len))) return hr;
        if (max_len < nv12.size()) {
            buffer->Unlock();
            return E_UNEXPECTED;
        }
        memcpy(destination, nv12.data(), nv12.size());
        buffer->Unlock();
        if (FAILED(hr = buffer->SetCurrentLength(static_cast<DWORD>(nv12.size())))) return hr;

        ComPtr<IMFSample> sample;
        if (FAILED(hr = MFCreateSample(&sample))) return hr;
        if (FAILED(hr = sample->AddBuffer(buffer.Get()))) return hr;
        if (FAILED(hr = sample->SetSampleTime(timestamp))) return hr;
        if (FAILED(hr = sample->SetSampleDuration(frame_duration))) return hr;

        hr = transform->ProcessInput(0, sample.Get(), 0);
        if (hr == MF_E_NOTACCEPTING) {
            if (FAILED(hr = collect_output(encoded))) return hr;
            hr = transform->ProcessInput(0, sample.Get(), 0);
        }
        if (FAILED(hr)) return hr;
        return collect_output(encoded);
    }

    ~ZhiroxEncoder() {
        if (transform) {
            (void)transform->ProcessMessage(MFT_MESSAGE_NOTIFY_END_OF_STREAM, 0);
            (void)transform->ProcessMessage(MFT_MESSAGE_NOTIFY_END_STREAMING, 0);
            transform.Reset();
        }
        codec.Reset();
        if (mf_started) MFShutdown();
        if (co_uninitialize) CoUninitialize();
    }
};

ZHIROX_ENCODER_API long zhirox_encoder_create(
    uint32_t width,
    uint32_t height,
    uint32_t fps,
    uint32_t bitrate,
    ZhiroxEncoder** out_encoder) {
    if (!out_encoder) return E_POINTER;
    *out_encoder = nullptr;
    ZhiroxEncoder* encoder = new (std::nothrow) ZhiroxEncoder();
    if (!encoder) return E_OUTOFMEMORY;
    const HRESULT hr = encoder->initialize(width, height, fps, bitrate);
    if (FAILED(hr)) {
        delete encoder;
        return hr;
    }
    *out_encoder = encoder;
    return S_OK;
}

ZHIROX_ENCODER_API void zhirox_encoder_destroy(ZhiroxEncoder* encoder) {
    delete encoder;
}

ZHIROX_ENCODER_API long zhirox_encoder_encode_bgra(
    ZhiroxEncoder* encoder,
    const uint8_t* bgra,
    uint32_t stride,
    int64_t timestamp_100ns,
    int force_keyframe,
    uint8_t* output,
    size_t output_capacity,
    size_t* out_size) {
    if (!encoder || !bgra || !out_size) return E_POINTER;
    *out_size = 0;
    std::vector<uint8_t> encoded;
    const HRESULT hr = encoder->encode(bgra, stride, timestamp_100ns, force_keyframe != 0, encoded);
    if (FAILED(hr)) return hr;
    *out_size = encoded.size();
    if (encoded.empty()) return S_OK;
    if (!output || output_capacity < encoded.size()) {
        return HRESULT_FROM_WIN32(ERROR_INSUFFICIENT_BUFFER);
    }
    memcpy(output, encoded.data(), encoded.size());
    return S_OK;
}

#else

ZHIROX_ENCODER_API long zhirox_encoder_create(uint32_t, uint32_t, uint32_t, uint32_t, ZhiroxEncoder**) { return -1; }
ZHIROX_ENCODER_API void zhirox_encoder_destroy(ZhiroxEncoder*) {}
ZHIROX_ENCODER_API long zhirox_encoder_encode_bgra(ZhiroxEncoder*, const uint8_t*, uint32_t, int64_t, int, uint8_t*, size_t, size_t*) { return -1; }

#endif
