#ifdef _WIN32

#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mftransform.h>
#include <mferror.h>
#include <codecapi.h>
#include <wrl/client.h>
#include <cstdio>

using Microsoft::WRL::ComPtr;

static void print_hr(const char* stage, HRESULT hr) {
    std::printf("%s: 0x%08lx%s\n", stage, static_cast<unsigned long>(hr), SUCCEEDED(hr) ? " PASS" : " FAIL");
}

int wmain() {
    HRESULT hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
    print_hr("CoInitializeEx", hr);
    const bool uninit = SUCCEEDED(hr);
    if (FAILED(hr) && hr != RPC_E_CHANGED_MODE) return 10;

    hr = MFStartup(MF_VERSION, MFSTARTUP_FULL);
    print_hr("MFStartup", hr);
    if (FAILED(hr)) {
        if (uninit) CoUninitialize();
        return 11;
    }

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
    hr = MFTEnumEx(MFT_CATEGORY_VIDEO_ENCODER, flags, nullptr, &output_info, &activations, &count);
    print_hr("MFTEnumEx(H264)", hr);
    std::printf("encoder_count=%u\n", count);
    if (FAILED(hr) || count == 0) {
        if (activations) CoTaskMemFree(activations);
        MFShutdown();
        if (uninit) CoUninitialize();
        return 12;
    }

    bool configured = false;
    HRESULT last = E_FAIL;
    for (UINT32 i = 0; i < count; ++i) {
        std::printf("candidate=%u\n", i);
        ComPtr<IMFTransform> transform;
        hr = activations[i]->ActivateObject(IID_PPV_ARGS(&transform));
        print_hr("  ActivateObject", hr);
        if (FAILED(hr) || !transform) { last = hr; continue; }

        ComPtr<IMFAttributes> attrs;
        hr = transform->GetAttributes(&attrs);
        print_hr("  GetAttributes", hr);
        if (SUCCEEDED(hr) && attrs) {
            UINT32 async_mft = 0;
            const HRESULT async_hr = attrs->GetUINT32(MF_TRANSFORM_ASYNC, &async_mft);
            print_hr("  Get MF_TRANSFORM_ASYNC", async_hr);
            std::printf("  async_mft=%u\n", async_mft);
            if (SUCCEEDED(async_hr) && async_mft) {
                const HRESULT unlock_hr = attrs->SetUINT32(MF_TRANSFORM_ASYNC_UNLOCK, TRUE);
                print_hr("  Set MF_TRANSFORM_ASYNC_UNLOCK", unlock_hr);
            }
            const HRESULT latency_hr = attrs->SetUINT32(MF_LOW_LATENCY, TRUE);
            print_hr("  Set MF_LOW_LATENCY", latency_hr);
        }

        ComPtr<IMFMediaType> output;
        hr = MFCreateMediaType(&output);
        print_hr("  MFCreateMediaType(output)", hr);
        if (FAILED(hr)) { last = hr; continue; }
        if (FAILED(hr = output->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video))) { print_hr("  output major", hr); last = hr; continue; }
        if (FAILED(hr = output->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_H264))) { print_hr("  output subtype", hr); last = hr; continue; }
        if (FAILED(hr = output->SetUINT32(MF_MT_AVG_BITRATE, 1500000))) { print_hr("  output bitrate", hr); last = hr; continue; }
        if (FAILED(hr = MFSetAttributeSize(output.Get(), MF_MT_FRAME_SIZE, 640, 480))) { print_hr("  output size", hr); last = hr; continue; }
        if (FAILED(hr = MFSetAttributeRatio(output.Get(), MF_MT_FRAME_RATE, 15, 1))) { print_hr("  output fps", hr); last = hr; continue; }
        if (FAILED(hr = MFSetAttributeRatio(output.Get(), MF_MT_PIXEL_ASPECT_RATIO, 1, 1))) { print_hr("  output aspect", hr); last = hr; continue; }
        if (FAILED(hr = output->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive))) { print_hr("  output interlace", hr); last = hr; continue; }
        if (FAILED(hr = output->SetUINT32(MF_MT_MPEG2_PROFILE, eAVEncH264VProfile_Base))) { print_hr("  output profile", hr); last = hr; continue; }
        hr = transform->SetOutputType(0, output.Get(), 0);
        print_hr("  SetOutputType", hr);
        if (FAILED(hr)) { last = hr; continue; }

        ComPtr<IMFMediaType> input;
        hr = MFCreateMediaType(&input);
        print_hr("  MFCreateMediaType(input)", hr);
        if (FAILED(hr)) { last = hr; continue; }
        if (FAILED(hr = input->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video))) { print_hr("  input major", hr); last = hr; continue; }
        if (FAILED(hr = input->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_NV12))) { print_hr("  input subtype", hr); last = hr; continue; }
        if (FAILED(hr = MFSetAttributeSize(input.Get(), MF_MT_FRAME_SIZE, 640, 480))) { print_hr("  input size", hr); last = hr; continue; }
        if (FAILED(hr = MFSetAttributeRatio(input.Get(), MF_MT_FRAME_RATE, 15, 1))) { print_hr("  input fps", hr); last = hr; continue; }
        if (FAILED(hr = MFSetAttributeRatio(input.Get(), MF_MT_PIXEL_ASPECT_RATIO, 1, 1))) { print_hr("  input aspect", hr); last = hr; continue; }
        if (FAILED(hr = input->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive))) { print_hr("  input interlace", hr); last = hr; continue; }
        hr = transform->SetInputType(0, input.Get(), 0);
        print_hr("  SetInputType", hr);
        if (FAILED(hr)) { last = hr; continue; }

        MFT_OUTPUT_STREAM_INFO stream{};
        hr = transform->GetOutputStreamInfo(0, &stream);
        print_hr("  GetOutputStreamInfo", hr);
        if (FAILED(hr)) { last = hr; continue; }
        std::printf("  output_flags=0x%08lx cbSize=%lu cbAlignment=%lu\n",
            static_cast<unsigned long>(stream.dwFlags),
            static_cast<unsigned long>(stream.cbSize),
            static_cast<unsigned long>(stream.cbAlignment));

        hr = transform->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0);
        print_hr("  COMMAND_FLUSH", hr);
        if (FAILED(hr)) { last = hr; continue; }
        hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);
        print_hr("  BEGIN_STREAMING", hr);
        if (FAILED(hr)) { last = hr; continue; }
        hr = transform->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0);
        print_hr("  START_OF_STREAM", hr);
        if (FAILED(hr)) { last = hr; continue; }

        configured = true;
        last = S_OK;
        break;
    }

    for (UINT32 i = 0; i < count; ++i) if (activations[i]) activations[i]->Release();
    CoTaskMemFree(activations);
    MFShutdown();
    if (uninit) CoUninitialize();

    std::printf("diagnostic_result=%s last=0x%08lx\n", configured ? "CONFIGURED" : "FAILED", static_cast<unsigned long>(last));
    return configured ? 0 : 20;
}

#else
int main() { return 0; }
#endif
