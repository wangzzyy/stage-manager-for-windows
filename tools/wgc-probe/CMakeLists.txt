cmake_minimum_required(VERSION 3.25)
project(wgc_probe LANGUAGES CXX)

add_executable(wgc-probe main.cpp)
target_compile_features(wgc-probe PRIVATE cxx_std_20)
target_compile_definitions(wgc-probe PRIVATE
  WIN32_LEAN_AND_MEAN
  NOMINMAX
  WINVER=0x0A00
  _WIN32_WINNT=0x0A00
)
target_link_libraries(wgc-probe PRIVATE d3d11 runtimeobject)
