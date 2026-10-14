#include "encoder.h"

#ifdef _WIN32
#include <windows.h>
#include <cstdint>
#include <cstdio>
#include <vector>

using CreateFn = long (*)(uint32_t, uint32_t, uint32_t, uint32_t, ZhiroxEncoder**);
using DestroyFn = void (*)(ZhiroxEncoder*);
using EncodeFn = long (*)(ZhiroxEncoder*, const uint8_t*, uint32_t, int64_t, int, uint8_t*, size_t, size_t*);

static bool failed(long hr) {
    return hr < 0;
}

static bool has_annex_b_start_code(const uint8_t* data, size_t n) {
    for (size_t i = 0; i + 3 < n; ++i) {
        if (data[i] == 0 && data[i + 1] == 0 && data[i + 2] == 1) return true;
        if (i + 4 < n && data[i] == 0 && data[i + 1] == 0 && data[i + 2] == 0 && data[i + 3] == 1) return true;
    }
    return false;
}

int wmain() {
    HMODULE module = LoadLibraryW(L"ZhiroxMFH264Encoder.dll");
    if (!module) {
        std::printf("LoadLibrary failed: %lu\n", GetLastError());
        return 10;
    }
    const auto create = reinterpret_cast<CreateFn>(GetProcAddress(module, "zhirox_encoder_create"));
    const auto destroy = reinterpret_cast<DestroyFn>(GetProcAddress(module, "zhirox_encoder_destroy"));
    const auto encode = reinterpret_cast<EncodeFn>(GetProcAddress(module, "zhirox_encoder_encode_bgra"));
    if (!create || !destroy || !encode) {
        std::printf("missing encoder exports\n");
        FreeLibrary(module);
        return 11;
    }

    constexpr uint32_t width = 640;
    constexpr uint32_t height = 480;
    constexpr uint32_t fps = 15;
    ZhiroxEncoder* encoder = nullptr;
    long hr = create(width, height, fps, 1500000, &encoder);
    if (failed(hr) || !encoder) {
        std::printf("encoder create failed: 0x%08lx\n", static_cast<unsigned long>(hr));
        FreeLibrary(module);
        return 12;
    }

    std::vector<uint8_t> bgra(static_cast<size_t>(width) * height * 4);
    for (uint32_t y = 0; y < height; ++y) {
        for (uint32_t x = 0; x < width; ++x) {
            const size_t p = (static_cast<size_t>(y) * width + x) * 4;
            bgra[p + 0] = static_cast<uint8_t>((x + y) & 0xff);
            bgra[p + 1] = static_cast<uint8_t>((2 * y) & 0xff);
            bgra[p + 2] = static_cast<uint8_t>((2 * x) & 0xff);
            bgra[p + 3] = 0xff;
        }
    }

    std::vector<uint8_t> output(2 * 1024 * 1024);
    size_t total_bytes = 0;
    bool annex_b = false;
    for (int frame = 0; frame < 12; ++frame) {
        size_t out_size = 0;
        const int64_t timestamp = static_cast<int64_t>(frame) * (10000000LL / fps);
        hr = encode(
            encoder,
            bgra.data(),
            width * 4,
            timestamp,
            frame == 0 ? 1 : 0,
            output.data(),
            output.size(),
            &out_size);
        if (failed(hr)) {
            std::printf("encode failed on frame %d: 0x%08lx required=%zu\n", frame, static_cast<unsigned long>(hr), out_size);
            destroy(encoder);
            FreeLibrary(module);
            return 13;
        }
        if (out_size > 0) {
            total_bytes += out_size;
            annex_b = annex_b || has_annex_b_start_code(output.data(), out_size);
        }
    }

    destroy(encoder);
    FreeLibrary(module);
    if (total_bytes == 0) {
        std::printf("encoder produced no H264 bytes\n");
        return 14;
    }
    if (!annex_b) {
        std::printf("H264 output did not expose Annex-B start codes\n");
        return 15;
    }
    std::printf("PASS: Media Foundation H264 encoder produced %zu bytes with Annex-B start codes\n", total_bytes);
    return 0;
}
#else
int main() { return 0; }
#endif
