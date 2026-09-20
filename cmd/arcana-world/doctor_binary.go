package main

import (
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
)

func (r *doctorReport) inspectBinary(topic, path string, required bool) {
	var arch string
	var libraries []string
	var err error
	switch runtime.GOOS {
	case "linux":
		var binary *elf.File
		binary, err = elf.Open(path)
		if err == nil {
			defer binary.Close()
			arch = map[elf.Machine]string{elf.EM_X86_64: "amd64", elf.EM_386: "386", elf.EM_AARCH64: "arm64", elf.EM_ARM: "arm", elf.EM_RISCV: "riscv64", elf.EM_PPC64: "ppc64", elf.EM_S390: "s390x"}[binary.Machine]
			if arch == "ppc64" && binary.Data == elf.ELFDATA2LSB {
				arch = "ppc64le"
			}
			libraries, err = binary.ImportedLibraries()
			for _, program := range binary.Progs {
				if program.Type != elf.PT_INTERP {
					continue
				}
				data, readErr := io.ReadAll(io.LimitReader(program.Open(), 4097))
				if readErr != nil || len(data) > 4096 {
					r.issue(required, topic, "cannot inspect ELF dynamic loader")
					continue
				}
				loader := strings.TrimRight(string(data), "\x00")
				info, statErr := os.Stat(loader)
				if statErr != nil || !info.Mode().IsRegular() {
					r.issue(required, topic, "ELF dynamic loader %q is missing or inaccessible", loader)
				}
			}
		}
	case "darwin":
		var binary *macho.File
		binary, err = macho.Open(path)
		if err != nil {
			fat, fatErr := macho.OpenFat(path)
			if fatErr == nil {
				defer fat.Close()
				for _, candidate := range fat.Arches {
					if doctorMachArch(candidate.Cpu) == runtime.GOARCH {
						binary, err = candidate.File, nil
						break
					}
				}
				if binary == nil {
					r.line("WARN", topic, "universal Mach-O helper has no architecture matching %s; emulation is not probed", runtime.GOARCH)
					return
				}
			}
		} else {
			defer binary.Close()
		}
		if err == nil {
			arch = doctorMachArch(binary.Cpu)
			libraries, err = binary.ImportedLibraries()
		}
	case "windows":
		var binary *pe.File
		binary, err = pe.Open(path)
		if err == nil {
			defer binary.Close()
			arch = map[uint16]string{pe.IMAGE_FILE_MACHINE_AMD64: "amd64", pe.IMAGE_FILE_MACHINE_I386: "386", pe.IMAGE_FILE_MACHINE_ARM64: "arm64"}[binary.Machine]
			libraries, err = binary.ImportedLibraries()
		}
	default:
		r.line("WARN", topic, "binary format and dependencies cannot be inspected on this platform")
		return
	}
	if err != nil {
		r.issue(required, topic, "helper is not a readable native binary or has invalid dependency metadata")
		return
	}
	if arch == "" {
		r.line("WARN", topic, "binary architecture is not recognized; compatibility unverified")
	} else if arch != runtime.GOARCH {
		r.line("WARN", topic, "helper architecture %s differs from process architecture %s; emulation is not probed", arch, runtime.GOARCH)
	} else {
		r.line("OK", topic, "native binary architecture matches %s", arch)
	}
	if len(libraries) > 0 {
		r.line("WARN", topic+" dependencies", "%q; dynamic loader search, transitive libraries, versions and signatures not verified", libraries)
	} else {
		r.line("WARN", topic+" dependencies", "no direct imports reported; runtime-loaded libraries and device access remain unverified")
	}
	info, buildErr := buildinfo.ReadFile(path)
	if buildErr != nil {
		r.line("WARN", topic+" build", "Go build metadata unavailable; helper feature support not verified")
		return
	}
	if topic == "overlay" && runtime.GOOS == "linux" {
		var cgo, tags string
		for _, setting := range info.Settings {
			if setting.Key == "CGO_ENABLED" {
				cgo = setting.Value
			}
			if setting.Key == "-tags" {
				tags = setting.Value
			}
		}
		if cgo == "0" || tags != "" && !slices.Contains(strings.FieldsFunc(tags, func(c rune) bool { return c == ',' || c == ' ' }), "wayland") {
			r.issue(required, topic+" build", "Linux overlay requires CGO_ENABLED=1 and the wayland build tag")
		} else if cgo != "1" || tags == "" {
			r.line("WARN", topic+" build", "Wayland/cgo build support cannot be confirmed from build metadata")
		}
	}
}

func doctorMachArch(cpu macho.Cpu) string {
	switch cpu {
	case macho.CpuAmd64:
		return "amd64"
	case macho.CpuArm64:
		return "arm64"
	case macho.Cpu386:
		return "386"
	case macho.CpuArm:
		return "arm"
	}
	return ""
}
