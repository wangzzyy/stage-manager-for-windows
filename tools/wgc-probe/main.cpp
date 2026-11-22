#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>
#include <windows.graphics.capture.interop.h>
#include <windows.graphics.directx.direct3d11.interop.h>

#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Graphics.Capture.h>
#include <winrt/Windows.Graphics.DirectX.h>
#include <winrt/Windows.Graphics.DirectX.Direct3D11.h>
#include <winrt/base.h>

#include <algorithm>
#include <atomic>
#include <chrono>
#include <cstdint>
#include <cwchar>
#include <iterator>
#include <cstdlib>
#include <iostream>
#include <string>
#include <thread>
#include <vector>

using winrt::Windows::Graphics::Capture::Direct3D11CaptureFramePool;
using winrt::Windows::Graphics::Capture::GraphicsCaptureItem;
using winrt::Windows::Graphics::Capture::GraphicsCaptureSession;
using winrt::Windows::Graphics::DirectX::DirectXPixelFormat;
using winrt::Windows::Graphics::DirectX::Direct3D11::IDirect3DDevice;

struct WindowEntry {
    HWND hwnd{};
    std::wstring title;
    std::wstring className;
    RECT rect{};
    DWORD pid{};
};

static std::wstring currentStage = L"initialization";

void log_stage(const wchar_t* message) {
    currentStage = message;
    std::wcout << L"[stage] " << message << L"\n" << std::flush;
}

BOOL CALLBACK collect_window(HWND hwnd, LPARAM param) {
    if (!IsWindowVisible(hwnd) || IsIconic(hwnd)) return TRUE;
    wchar_t title[512]{};
    wchar_t className[256]{};
    GetWindowTextW(hwnd, title, static_cast<int>(std::size(title)));
    if (title[0] == L'\\0') return TRUE;
    GetClassNameW(hwnd, className, static_cast<int>(std::size(className)));
    WindowEntry entry{};
    entry.hwnd = hwnd;
    entry.title = title;
    entry.className = className;
    GetWindowRect(hwnd, &entry.rect);
    GetWindowThreadProcessId(hwnd, &entry.pid);
    reinterpret_cast<std::vector<WindowEntry>*>(param)->push_back(std::move(entry));
    return TRUE;
}

std::vector<WindowEntry> windows() {
    std::vector<WindowEntry> result;
    EnumWindows(collect_window, reinterpret_cast<LPARAM>(&result));
    return result;
}

void print_windows(const std::vector<WindowEntry>& entries) {
    std::wcout << L"Visible, non-minimized top-level windows (off-screen windows are included):\n";
    for (const auto& e : entries) {
        std::wcout << L"  HWND=0x" << std::hex << reinterpret_cast<std::uintptr_t>(e.hwnd)
                   << std::dec << L" PID=" << e.pid
                   << L" rect=(" << e.rect.left << L"," << e.rect.top << L","
                   << e.rect.right << L"," << e.rect.bottom << L") "
                   << L"class=\"" << e.className << L"\" title=\"" << e.title << L"\"\n";
    }
}

IDirect3DDevice create_capture_device() {
    log_stage(L"creating D3D11 hardware device");
    winrt::com_ptr<ID3D11Device> device;
    winrt::com_ptr<ID3D11DeviceContext> context;
    D3D_FEATURE_LEVEL level{};
    winrt::check_hresult(D3D11CreateDevice(
        nullptr, D3D_DRIVER_TYPE_HARDWARE, nullptr, D3D11_CREATE_DEVICE_BGRA_SUPPORT,
        nullptr, 0, D3D11_SDK_VERSION, device.put(), &level, context.put()));

    log_stage(L"converting D3D11 device to WinRT IDirect3DDevice");
    winrt::com_ptr<IDXGIDevice> dxgiDevice;
    winrt::check_hresult(device->QueryInterface(IID_PPV_ARGS(dxgiDevice.put())));
    winrt::com_ptr<IInspectable> inspectable;
    winrt::check_hresult(CreateDirect3D11DeviceFromDXGIDevice(dxgiDevice.get(), inspectable.put()));
    return inspectable.as<IDirect3DDevice>();
}

GraphicsCaptureItem create_capture_item(HWND hwnd) {
    log_stage(L"creating GraphicsCaptureItem for HWND");
    auto interop = winrt::get_activation_factory<GraphicsCaptureItem, IGraphicsCaptureItemInterop>();
    GraphicsCaptureItem item{ nullptr };
    winrt::check_hresult(interop->CreateForWindow(
        hwnd, winrt::guid_of<GraphicsCaptureItem>(), winrt::put_abi(item)));
    return item;
}

int wmain(int argc, wchar_t** argv) {
    try {
        winrt::init_apartment(winrt::apartment_type::multi_threaded);
        HWND target = nullptr;
        int seconds = 20;
        for (int i = 1; i < argc; ++i) {
            const std::wstring arg = argv[i];
            if (arg == L"--hwnd" && i + 1 < argc) {
                target = reinterpret_cast<HWND>(std::wcstoull(argv[++i], nullptr, 0));
            } else if (arg == L"--seconds" && i + 1 < argc) {
                seconds = std::clamp(_wtoi(argv[++i]), 1, 600);
            } else if (arg == L"--help") {
                std::wcout << L"Usage: wgc-probe.exe [--hwnd 0x1234] [--seconds 20]\n"
                              L"Without --hwnd, lists windows and prompts for a handle.\n";
                return 0;
            }
        }

        log_stage(L"checking Windows Graphics Capture support");
        if (!GraphicsCaptureSession::IsSupported()) {
            std::wcerr << L"WGC is not supported on this system.\n";
            return 2;
        }
        if (!target) {
            auto entries = windows();
            print_windows(entries);
            std::wcout << L"Enter the HWND to capture (hex, e.g. 0x001A03F2): " << std::flush;
            std::wstring value;
            std::wcin >> value;
            target = reinterpret_cast<HWND>(std::wcstoull(value.c_str(), nullptr, 0));
        }
        if (!target || !IsWindow(target) || IsIconic(target)) {
            std::wcerr << L"Target HWND is invalid or minimized. A parked window should not be minimized.\n";
            return 2;
        }

        DWORD pid = 0;
        GetWindowThreadProcessId(target, &pid);
        RECT rect{};
        GetWindowRect(target, &rect);
        wchar_t title[512]{};
        GetWindowTextW(target, title, static_cast<int>(std::size(title)));
        std::wcout << L"Capturing HWND=0x" << std::hex << reinterpret_cast<std::uintptr_t>(target)
                   << std::dec << L" PID=" << pid << L" title=\"" << title << L"\" rect=("
                   << rect.left << L"," << rect.top << L"," << rect.right << L"," << rect.bottom
                   << L") for " << seconds << L" seconds.\n" << std::flush;

        auto item = create_capture_item(target);
        const auto itemSize = item.Size();
        std::wcout << L"[info] capture item size=" << itemSize.Width << L"x" << itemSize.Height << L"\n" << std::flush;
        auto device = create_capture_device();

        log_stage(L"creating Direct3D11CaptureFramePool");
        auto pool = Direct3D11CaptureFramePool::CreateFreeThreaded(
            device, DirectXPixelFormat::B8G8R8A8UIntNormalized, 3, itemSize);
        log_stage(L"creating GraphicsCaptureSession");
        auto session = pool.CreateCaptureSession(item);

        std::atomic<std::uint64_t> frames{ 0 };
        std::atomic<int> lastWidth{ 0 };
        std::atomic<int> lastHeight{ 0 };
        log_stage(L"registering FrameArrived handler");
        auto token = pool.FrameArrived([&](const Direct3D11CaptureFramePool& sender, const auto&) {
            for (auto frame = sender.TryGetNextFrame(); frame; frame = sender.TryGetNextFrame()) {
                const auto size = frame.ContentSize();
                lastWidth.store(size.Width, std::memory_order_relaxed);
                lastHeight.store(size.Height, std::memory_order_relaxed);
                frames.fetch_add(1, std::memory_order_relaxed);
            }
        });

        log_stage(L"starting capture session");
        session.StartCapture();
        log_stage(L"capture running; collecting frames");
        const auto started = std::chrono::steady_clock::now();
        std::uint64_t previous = 0;
        while (std::chrono::steady_clock::now() - started < std::chrono::seconds(seconds)) {
            std::this_thread::sleep_for(std::chrono::seconds(1));
            const auto current = frames.load(std::memory_order_relaxed);
            std::wcout << L"frames=" << current << L" delta=" << (current - previous)
                       << L" size=" << lastWidth.load() << L"x" << lastHeight.load() << L"\n" << std::flush;
            previous = current;
            if (!IsWindow(target)) {
                std::wcerr << L"Target window was destroyed during capture.\n";
                break;
            }
        }

        log_stage(L"stopping capture and releasing resources");
        pool.FrameArrived(token);
        session.Close();
        pool.Close();
        const auto elapsed = std::chrono::duration<double>(std::chrono::steady_clock::now() - started).count();
        const auto total = frames.load();
        std::wcout << L"RESULT total_frames=" << total << L" elapsed_seconds=" << elapsed
                   << L" average_fps=" << (elapsed > 0 ? total / elapsed : 0) << L"\n";
        return total > 0 ? 0 : 3;
    } catch (const winrt::hresult_error& e) {
        std::wcerr << L"WGC probe failed during [" << currentStage << L"]: 0x" << std::hex
                   << static_cast<std::uint32_t>(e.code()) << L" " << e.message().c_str() << L"\n" << std::flush;
        return 1;
    } catch (const std::exception& e) {
        std::wcerr << L"WGC probe failed during [" << currentStage << L"]: "
                   << e.what() << L"\n" << std::flush;
        return 1;
    }
}
