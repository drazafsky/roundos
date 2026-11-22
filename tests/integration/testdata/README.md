# Unmodified 0.5.0 compatibility source

`v050-src.tar.gz` contains all 41 non-test Go source files and `go.mod` from
the original `roundos-0.5.0.zip`. No selected source bytes were changed.
Original ZIP SHA-256:

`06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75`

Fixture SHA-256:

`63a3cd7886b44069c610e04a5debf7bda158477a593690a0eda0b3926e5b2659`

The compatibility test checks this digest and every extracted source against
`scripts/m0/BASELINE-SOURCE.SHA256`, then compiles separate old executables.
Its additional old-manager harness is test code; the old manager, SDK, wire
types and applications remain unchanged. These tests exercise protocol and
process behavior. Physical display, timing, installation and rollback remain
Pi acceptance gates.
