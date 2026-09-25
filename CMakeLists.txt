cmake_minimum_required(VERSION 3.20)

project(DaedalusVM
    VERSION 0.1.0
    DESCRIPTION "A lightweight custom virtual machine loader and assembler"
    LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Default to a Release build when the generator is single-config and the user
# did not pick one (e.g. plain `cmake -B build` with Ninja/Makefiles).
if(NOT CMAKE_BUILD_TYPE AND NOT CMAKE_CONFIGURATION_TYPES)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "" FORCE)
endif()

# ---------------------------------------------------------------------------
# VM loader (C++)
# ---------------------------------------------------------------------------
add_executable(Ap3xVMLoader Ap3xVMLoader.cpp)

if(MSVC)
    target_compile_options(Ap3xVMLoader PRIVATE /W4)
    # fopen() is standard C; silence MSVC's non-portable "use fopen_s" warning.
    target_compile_definitions(Ap3xVMLoader PRIVATE _CRT_SECURE_NO_WARNINGS)
else()
    target_compile_options(Ap3xVMLoader PRIVATE -Wall -Wextra)
endif()

# ---------------------------------------------------------------------------
# Go assembler (compiler/VMCompiler.exe), built as part of the default build
# when a Go toolchain is available.
# ---------------------------------------------------------------------------
find_program(GO_EXECUTABLE go)
if(GO_EXECUTABLE)
    set(ASSEMBLER_DIR "${CMAKE_CURRENT_SOURCE_DIR}/compiler")
    set(ASSEMBLER_EXE "${ASSEMBLER_DIR}/VMCompiler${CMAKE_EXECUTABLE_SUFFIX}")
    file(GLOB ASSEMBLER_SOURCES CONFIGURE_DEPENDS "${ASSEMBLER_DIR}/*.go")

    add_custom_command(
        OUTPUT "${ASSEMBLER_EXE}"
        COMMAND "${GO_EXECUTABLE}" build -o "${ASSEMBLER_EXE}" .
        WORKING_DIRECTORY "${ASSEMBLER_DIR}"
        DEPENDS ${ASSEMBLER_SOURCES}
        COMMENT "Building Go assembler (VMCompiler)"
        VERBATIM)

    add_custom_target(assembler ALL DEPENDS "${ASSEMBLER_EXE}")
else()
    message(WARNING
        "Go toolchain not found: skipping the VMCompiler assembler target. "
        "Install Go and re-run CMake to build it.")
endif()
