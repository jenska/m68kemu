# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/SemVer).

## [1.2.3] - 2026-04-02

### Fixed
- Corrected 68000 Line-A and Line-F exception frames to stack the trapping opcode address
- Expanded validation around USP moves, privilege traps, and supervisor/user stack bank switching

## [1.2.1] - 2026-03-28

### Fixed
- Fixed MOVEM control-mode sequencing
- Added support for address-register sources in ADD and SUB instructions

## [1.2.2] - 2026-03-29

### Fixed
- Enhanced arithmetic and bit operation handling with immediate values
- Added new tests for instruction behavior

## [1.2.0] - 2026-03-28

### Changed
- Updated m68kasm to v1.3.0, adding support for $ in expressions and .w/.l label suffixes
- Updated m68kdasm to v1.0.1, fixing MOVEM register decoding and SWAP instruction handling
- Fixed PC-relative addressing calculation in PEA instruction test

### Dependencies
- github.com/jenska/m68kasm v1.3.0
- github.com/jenska/m68kdasm v1.0.1

## [1.1.0] - 2024-12-01

### Added
- Initial release

## [1.0.1] - 2024-11-15

### Fixed
- Bug fixes

## [1.0.0] - 2024-11-01

### Added
- Initial release
