#!/bin/bash
# Build script for Argus Configuration Framework
# Supports reproducible builds and debug symbol preservation

set -e

BINARY_NAME="argus"
BUILD_DIR="./bin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Parse command line arguments
STRIP_SYMBOLS=false
VERBOSE=false
REPRODUCIBLE=false

while [[ $# -gt 0 ]]; do
  case $1 in
    -s|--strip)
      STRIP_SYMBOLS=true
      shift
      ;;
    -r|--reproducible)
      REPRODUCIBLE=true
      shift
      ;;
    -v|--verbose)
      VERBOSE=true
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [OPTIONS]"
      echo "Options:"
      echo "  -s, --strip        Strip debug symbols (production build)"
      echo "  -r, --reproducible Enable reproducible builds (deterministic output)"
      echo "  -v, --verbose      Enable verbose output"
      echo "  -h, --help         Show this help message"
      echo ""
      echo "By default, debug symbols are preserved for development."
      exit 0
      ;;
    *)
      echo "Unknown option $1"
      exit 1
      ;;
  esac
done

# Create build directory
mkdir -p "$BUILD_DIR"

# Build flags
BUILD_FLAGS=""
LDFLAGS=""

if [ "$STRIP_SYMBOLS" = true ]; then
  LDFLAGS="-w -s"
fi

if [ "$REPRODUCIBLE" = true ]; then
  if [ -n "$LDFLAGS" ]; then
    LDFLAGS="$LDFLAGS -buildid="
  else
    LDFLAGS="-buildid="
  fi
  export CGO_ENABLED=0
  export SOURCE_DATE_EPOCH=1
  echo -e "${YELLOW}Building ${BINARY_NAME} (reproducible build)...${NC}"
elif [ "$STRIP_SYMBOLS" = true ]; then
  echo -e "${YELLOW}Building ${BINARY_NAME} (production - debug symbols stripped)...${NC}"
else
  echo -e "${YELLOW}Building ${BINARY_NAME} (development - debug symbols preserved)...${NC}"
fi

if [ -n "$LDFLAGS" ]; then
  BUILD_FLAGS="-ldflags='$LDFLAGS'"
fi

# Build command with reproducible builds
REPRODUCIBLE_FLAGS="-trimpath"
if [ "$STRIP_SYMBOLS" = true ]; then
  BUILD_CMD="go build -trimpath -ldflags='-w -s -buildid=' -o $BUILD_DIR/$BINARY_NAME ."
else
  BUILD_CMD="go build -trimpath -ldflags='-buildid=' -o $BUILD_DIR/$BINARY_NAME ."
fi

if [ "$VERBOSE" = true ]; then
  echo "$BUILD_CMD"
fi

eval "$BUILD_CMD"

echo -e "${GREEN}Build completed successfully!${NC}"
echo "Binary location: $BUILD_DIR/$BINARY_NAME"

# Show file size
if command -v ls >/dev/null 2>&1; then
  echo "Binary size: $(ls -lh "$BUILD_DIR/$BINARY_NAME" | awk '{print $5}')"
fi

# Show debug symbol status
if command -v file >/dev/null 2>&1; then
  if file "$BUILD_DIR/$BINARY_NAME" | grep -q "not stripped"; then
    echo -e "${GREEN}Debug symbols: Present${NC}"
  else
    echo -e "${YELLOW}Debug symbols: Stripped${NC}"
  fi
fi