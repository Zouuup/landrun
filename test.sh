#!/bin/bash

# Check if we should keep the binary
KEEP_BINARY=false
USE_SYSTEM_BINARY=false
NO_BUILD=false
# Whether we should try to do network calls during testing.
INTERNET_ACCESS=true

while [ "$#" -gt 0 ]; do
    case "$1" in
        "--keep-binary")
            KEEP_BINARY=true
            shift
            ;;
        "--use-system")
            USE_SYSTEM_BINARY=true
            shift
            ;;
        "--no-build")
            NO_BUILD=true
            shift
            ;;
        "--offline")
            INTERNET_ACCESS=false
            shift
            ;;
        *)
            echo "Unknown parameter: $1"
            exit 1
            ;;
    esac
done

# Don't exit on error, we'll handle errors in the run_test function
set +e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Build the binary if not using system binary
if [ "$USE_SYSTEM_BINARY" = false ]; then
	if [ "$NO_BUILD" = false ]; then
		print_status "Building landrun binary..."
		go build -o landrun cmd/landrun/main.go
		if [ $? -ne 0 ]; then
			print_error "Failed to build landrun binary"
			exit 1
		fi
		print_success "Binary built successfully"
	else
		print_success "Using already built landrun binary"
	fi
fi

# Create test directories
TEST_DIR="test_env"
RO_DIR="$TEST_DIR/ro"
RO_DIR_NESTED_RO="$RO_DIR/ro_nested_ro_1"
RO_DIR_NESTED_RW="$RO_DIR/ro_nested_rw_1"
RO_DIR_NESTED_EXEC="$RO_DIR/ro_nested_exec"

RW_DIR="$TEST_DIR/rw"
RW_DIR_NESTED_RO="$RW_DIR/rw_nested_ro_1"
RW_DIR_NESTED_RW="$RW_DIR/rw_nested_rw_1"
RW_DIR_NESTED_EXEC="$RW_DIR/rw_nested_exec"

EXEC_DIR="$TEST_DIR/exec"
NESTED_DIR="$TEST_DIR/nested/path/deep"

print_status "Setting up test environment..."
rm -rf "$TEST_DIR"
mkdir -p "$RO_DIR" "$RW_DIR" "$EXEC_DIR" "$NESTED_DIR" "$RO_DIR_NESTED_RO" "$RO_DIR_NESTED_RW" "$RO_DIR_NESTED_EXEC" "$RW_DIR_NESTED_RO" "$RW_DIR_NESTED_RW" "$RW_DIR_NESTED_EXEC"

# Create test files
echo "readonly content" > "$RO_DIR/test.txt"
echo "readwrite content" > "$RW_DIR/test.txt"
echo "nested content" > "$NESTED_DIR/test.txt"
echo "#!/bin/bash" > "$EXEC_DIR/test.sh"
echo "echo 'executable content'" >> "$EXEC_DIR/test.sh"
chmod +x "$EXEC_DIR/test.sh"
cp $EXEC_DIR/test.sh $EXEC_DIR/test2.sh

cp "$RO_DIR/test.txt" "$RO_DIR_NESTED_RO/test.txt"
cp "$RO_DIR/test.txt" "$RW_DIR_NESTED_RO/test.txt"

cp "$RW_DIR/test.txt" "$RO_DIR_NESTED_RW/test.txt"
cp "$RW_DIR/test.txt" "$RW_DIR_NESTED_RW/test.txt"

cp "$EXEC_DIR/test.sh" "$RO_DIR_NESTED_EXEC/test.sh"
cp "$EXEC_DIR/test.sh" "$RW_DIR_NESTED_EXEC/test.sh"
cp "$EXEC_DIR/test.sh" "$RO_DIR_NESTED_RO/test.sh"
cp "$EXEC_DIR/test.sh" "$RW_DIR_NESTED_RO/test.sh"
cp "$EXEC_DIR/test.sh" "$RO_DIR_NESTED_RW/test.sh"
cp "$EXEC_DIR/test.sh" "$RW_DIR_NESTED_RW/test.sh"

# Create a script in RW dir to test execution in RW dirs
echo "#!/bin/bash" > "$RW_DIR/rw_script.sh"
echo "echo 'this script is in a read-write directory'" >> "$RW_DIR/rw_script.sh"
chmod +x "$RW_DIR/rw_script.sh"

# Probe Landlock ABI version (0 if unavailable). Used by strict-mode tests.
LANDLOCK_ABI=$(go run github.com/landlock-lsm/go-landlock/cmd/landlock-abi-version@v0.9.0 2>/dev/null || echo 0)
LANDLOCK_ABI=$(echo "$LANDLOCK_ABI" | tr -d '[:space:]')
print_status "Detected Landlock ABI: ${LANDLOCK_ABI}"

# Function to run a test case
run_test() {
    local name="$1"
    local cmd="$2"
    local expected_exit="$3"
    
    # Replace ./landrun with landrun if using system binary
    if [ "$USE_SYSTEM_BINARY" = true ]; then
        cmd="${cmd//.\/landrun/landrun}"
    fi

    # The default target is Landlock ABI v9. Inject --best-effort so the suite
    # gracefully degrades and runs on kernels below v9. This only affects which
    # ABI level is targeted; the allow/deny semantics being tested are unchanged.
    cmd="${cmd/landrun /landrun --best-effort }"

    print_status "Running test: $name"
    eval "$cmd"
    local exit_code=$?
    
    if [ $exit_code -eq $expected_exit ]; then
        print_success "Test passed: $name"
        return 0
    else
        print_error "Test failed: $name (expected exit $expected_exit, got $exit_code)"
        exit 1
    fi
}

# Like run_test but does NOT inject --best-effort (strict ABI target).
run_test_strict() {
    local name="$1"
    local cmd="$2"
    local expected_exit="$3"

    if [ "$USE_SYSTEM_BINARY" = true ]; then
        cmd="${cmd//.\/landrun/landrun}"
    fi

    print_status "Running test (strict): $name"
    eval "$cmd"
    local exit_code=$?

    if [ $exit_code -eq $expected_exit ]; then
        print_success "Test passed: $name"
        return 0
    else
        print_error "Test failed: $name (expected exit $expected_exit, got $exit_code)"
        exit 1
    fi
}

# Test cases
print_status "Starting test cases..."

# Basic access tests
run_test "Read-only access to file" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- cat $RO_DIR/test.txt" \
    0

run_test "Read-only access to nested file" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- cat $RO_DIR_NESTED_RO/test.txt" \
    0

run_test "Write access to nested directory writable nested in read-only directory" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --rw $RO_DIR_NESTED_RW -- touch $RO_DIR_NESTED_RW/created_file" \
    0

run_test "Write access to nested file writable nested in read-only directory" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --rw $RO_DIR_NESTED_RW/created_file -- touch $RO_DIR_NESTED_RW/created_file" \
    0

run_test "Read-write access to file" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --rw $RW_DIR touch $RW_DIR/new.txt" \
    0

run_test "No write access to read-only directory" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --rw $RW_DIR touch $RO_DIR/new.txt" \
    1

# Executable permission tests
run_test "Execute access with rox flag" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rox $EXEC_DIR -- $EXEC_DIR/test.sh" \
    0

run_test "Execute access with rox flag on file" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rox $EXEC_DIR/test.sh -- $EXEC_DIR/test.sh" \
    0

run_test "Execute access with rox flag on a file that is executable in same directory that one is allowed" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rox $EXEC_DIR/test.sh -- $EXEC_DIR/test2.sh" \
    1

run_test "Execute a file with --add-exec flag" \
    "./landrun --log-level debug --add-exec --rox /usr --ro /lib --ro /lib64 --rox $EXEC_DIR/test.sh -- $EXEC_DIR/test2.sh" \
    0

run_test "Execute a file with --add-exec and --ldd flag" \
    "./landrun --log-level debug --add-exec --ldd -- $(which true)" \
    0


run_test "No execute access with just ro flag" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $EXEC_DIR -- $EXEC_DIR/test.sh" \
    1

run_test "Execute access in read-write directory" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rwx $RW_DIR -- $RW_DIR/rw_script.sh" \
    0

run_test "No execute access in read-write directory without rwx" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rw $RW_DIR -- $RW_DIR/rw_script.sh" \
    1

# Directory traversal tests
run_test "Directory traversal with root access" \
    "./landrun --log-level debug --rox / -- ls /usr" \
    0

run_test "Deep directory traversal" \
    "./landrun --log-level debug --rox / -- ls $NESTED_DIR" \
    0

# Multiple paths and complex specifications
run_test "Multiple read paths" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --ro $NESTED_DIR -- cat $NESTED_DIR/test.txt" \
    0

run_test "Comma-separated paths" \
    "./landrun --log-level debug --rox /usr --ro /lib,/lib64,$RO_DIR -- cat $RO_DIR/test.txt" \
    0

# System command tests
run_test "Simple system command" \
    "./landrun --log-level debug --rox /usr --ro  /etc -- whoami" \
    0

run_test "System command with arguments" \
    "./landrun --log-level debug --rox / -- ls -la /usr/bin" \
    0

# Edge cases
run_test "Non-existent read-only path" \
    "./landrun --log-level debug --ro /usr --ro /lib --ro /lib64 --ro /nonexistent/path -- ls" \
    1

run_test "No configuration" \
    "./landrun --log-level debug -- ls /" \
    1

# Process creation and redirection tests
run_test "Process creation with pipe" \
    "./landrun --log-level debug --rox / --env PATH -- bash -c 'ls /usr | grep bin'" \
    0

run_test "File redirection" \
    "./landrun --log-level debug --rox / --rw $RW_DIR --env PATH -- bash -c 'ls /usr > $RW_DIR/output.txt && cat $RW_DIR/output.txt'" \
    0

# Network restrictions tests (if kernel supports it)
$INTERNET_ACCESS && run_test "TCP connection without permission" \
    "./landrun --log-level debug --rox /usr --ro / -- curl -s --connect-timeout 2 https://example.com" \
    7

$INTERNET_ACCESS && run_test "TCP connection with permission" \
    "./landrun --log-level debug --rox /usr --ro / --connect-tcp 443 -- curl -s --connect-timeout 2 https://example.com" \
    0

# Environment isolation tests
export TEST_ENV_VAR="test_value_123"
run_test "Environment isolation" \
    "./landrun --log-level debug --rox /usr --ro / -- bash -c 'echo \$TEST_ENV_VAR'" \
    0

run_test "Environment isolation (no variables should be passed)" \
    "./landrun --log-level debug --rox /usr --ro / -- bash -c '[[ -z \$TEST_ENV_VAR ]] && echo \"No env var\" || echo \$TEST_ENV_VAR'" \
    0

run_test "Passing specific environment variable" \
    "./landrun --log-level debug --rox /usr --ro / --env TEST_ENV_VAR --env PATH -- bash -c 'echo \$TEST_ENV_VAR | grep \"test_value_123\"'" \
    0

run_test "Passing custom environment variable" \
    "./landrun --log-level debug --rox /usr --ro / --env CUSTOM_VAR=custom_value --env PATH -- bash -c 'echo \$CUSTOM_VAR | grep \"custom_value\"'" \
    0

# Combining different permission types
run_test "Mixed permissions" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rox $EXEC_DIR --rwx $RW_DIR --env PATH -- bash -c '$EXEC_DIR/test.sh > $RW_DIR/output.txt && cat $RW_DIR/output.txt'" \
    0

# Specific regression tests for bugs we fixed
run_test "Root path traversal regression test" \
    "./landrun --log-level debug --rox /usr -- $(which ls) /usr" \
    0

run_test "Execute from read-only paths regression test" \
    "./landrun --log-level debug --rox /usr --ro /usr/bin -- $(which id)" \
    0

run_test "Unrestricted filesystem access" \
    "./landrun --log-level debug --unrestricted-filesystem ls /usr" \
    0

$INTERNET_ACCESS && run_test "Unrestricted network access" \
    "./landrun --log-level debug --unrestricted-network --rox / -- curl -s --connect-timeout 2 http://kernel.org" \
    0

run_test "Restricted filesystem access" \
    "./landrun --log-level debug ls /usr" \
    1

$INTERNET_ACCESS && run_test "Restricted network access" \
    "./landrun --log-level debug --rox / -- curl -s --connect-timeout 2 http://kernel.org" \
    7

# New feature tests (Landlock V6-V9 / go-landlock v0.9.0)
run_test "Ignore missing path with --ignore-missing" \
    "./landrun --log-level debug --ignore-missing --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --ro /nonexistent/path -- cat $RO_DIR/test.txt" \
    0

run_test "Missing path without --ignore-missing still fails" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro /nonexistent/path -- cat $RO_DIR/test.txt" \
    1

run_test "Unrestricted IPC scoping smoke test" \
    "./landrun --log-level debug --unrestricted-scoped --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- cat $RO_DIR/test.txt" \
    0

run_test "UNIX socket path allowed with --unix" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --unix $RO_DIR/test.txt -- cat $RO_DIR/test.txt" \
    0

# --- CLI / startup edge cases ---
run_test "Missing command to run" \
    "./landrun --log-level error" \
    1

run_test "Unknown binary fails" \
    "./landrun --log-level error --rox /usr -- /nonexistent/landrun-binary-xyz" \
    1

run_test "Missing env key is omitted" \
    "./landrun --log-level debug --rox /usr --ro / --env LANDRUN_MISSING_ENV_KEY_XYZ --env PATH -- bash -c '[[ -z \$LANDRUN_MISSING_ENV_KEY_XYZ ]]'" \
    0

run_test "LANDRUN_LOG_LEVEL env smoke" \
    "LANDRUN_LOG_LEVEL=debug ./landrun --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- cat $RO_DIR/test.txt" \
    0

# --- Filesystem edge cases ---
# Create a dedicated rwx file for single-file exec tests
cp "$RW_DIR/rw_script.sh" "$RW_DIR/rwx_file.sh"
chmod +x "$RW_DIR/rwx_file.sh"

run_test "rwx on a single file allows execution" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rwx $RW_DIR/rwx_file.sh -- $RW_DIR/rwx_file.sh" \
    0

run_test "Overwrite of read-only file is denied" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- bash -c 'echo overwrite > $RO_DIR/test.txt'" \
    1

run_test "Truncate of read-only file is denied" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- bash -c ': > $RO_DIR/test.txt'" \
    1

run_test "Delete file under rw is allowed" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rw $RW_DIR --env PATH -- bash -c 'echo delme > $RW_DIR/todelete.txt && rm $RW_DIR/todelete.txt'" \
    0

run_test "Delete file under ro is denied" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --env PATH -- rm $RO_DIR/test.txt" \
    1

run_test "Execute from nested exec dir under ro parent" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --rox $RO_DIR_NESTED_EXEC -- $RO_DIR_NESTED_EXEC/test.sh" \
    0

run_test "Execute denied from nested exec dir with only ro parent" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- $RO_DIR_NESTED_EXEC/test.sh" \
    1

run_test "ldd without add-exec still resolves libraries for true" \
    "./landrun --log-level debug --ldd --add-exec -- $(which true)" \
    0

# Just --ldd: libraries get --rox but the binary itself may still need add-exec.
# Document expected behavior: without --add-exec, executing true requires the binary path.
TRUE_BIN=$(which true)
run_test "ldd alone without binary path may fail to exec" \
    "./landrun --log-level debug --ldd -- $TRUE_BIN" \
    1

run_test "ldd with explicit rox of binary succeeds" \
    "./landrun --log-level debug --ldd --rox $TRUE_BIN -- $TRUE_BIN" \
    0

# --- Local network tests (work offline) ---
# Use system python3 (not pyenv shims) so --ldd/--add-exec can resolve deps.
PYTHON=/usr/bin/python3
BIND_PORT=18765
BIND_PORT_OTHER=18766
PY_LANDRUN="./landrun --log-level debug --add-exec --ldd --rox /usr --ro /etc"

run_test "TCP bind allowed on permitted port" \
    "$PY_LANDRUN --bind-tcp $BIND_PORT -- $PYTHON -c \"
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', $BIND_PORT))
s.close()
\"" \
    0

run_test "TCP bind denied on non-permitted port" \
    "$PY_LANDRUN --bind-tcp $BIND_PORT -- $PYTHON -c \"
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    s.bind(('127.0.0.1', $BIND_PORT_OTHER))
except OSError:
    sys.exit(1)
sys.exit(0)
\"" \
    1

run_test "Comma-separated bind-tcp ports" \
    "$PY_LANDRUN --bind-tcp $BIND_PORT,$BIND_PORT_OTHER -- $PYTHON -c \"
import socket
for p in ($BIND_PORT, $BIND_PORT_OTHER):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(('127.0.0.1', p))
    s.close()
\"" \
    0

# Local connect allow/deny: start a listener outside the sandbox, connect from inside.
$PYTHON -c "
import socket, time, os, signal
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', $BIND_PORT))
s.listen(1)
os.write(1, b'ready\n')
conn, _ = s.accept()
conn.close()
s.close()
" > "$TEST_DIR/listener.out" 2>&1 &
LISTENER_PID=$!
# Wait until listener is ready
for i in 1 2 3 4 5 6 7 8 9 10; do
    if grep -q ready "$TEST_DIR/listener.out" 2>/dev/null; then break; fi
    sleep 0.1
done

run_test "TCP connect allowed to local permitted port" \
    "$PY_LANDRUN --connect-tcp $BIND_PORT -- $PYTHON -c \"
import socket
s = socket.create_connection(('127.0.0.1', $BIND_PORT), timeout=2)
s.close()
\"" \
    0

wait $LISTENER_PID 2>/dev/null || true

# Deny connect to a port that has no connect-tcp rule (listener not needed; connect fails at landlock)
run_test "TCP connect denied to non-permitted local port" \
    "$PY_LANDRUN --connect-tcp $BIND_PORT -- $PYTHON -c \"
import socket, sys
try:
    s = socket.create_connection(('127.0.0.1', $BIND_PORT_OTHER), timeout=1)
    s.close()
    sys.exit(0)
except OSError:
    sys.exit(1)
\"" \
    1

run_test "Unrestricted filesystem still restricts network without connect-tcp" \
    "./landrun --log-level debug --unrestricted-filesystem --add-exec --ldd -- $PYTHON -c \"
import socket, sys
try:
    s = socket.create_connection(('127.0.0.1', $BIND_PORT_OTHER), timeout=1)
    s.close()
    sys.exit(0)
except OSError:
    sys.exit(1)
\"" \
    1

# --- Unrestricted / domain combos ---
run_test "All domains unrestricted is a no-op sandbox" \
    "./landrun --log-level debug --unrestricted-filesystem --unrestricted-network --unrestricted-scoped -- echo ok" \
    0

run_test "unix paths ignored when filesystem unrestricted" \
    "./landrun --log-level debug --unrestricted-filesystem --unix /run/does-not-matter.sock -- echo ok" \
    0

run_test "FS restricted with net and scoped unrestricted" \
    "./landrun --log-level debug --unrestricted-network --unrestricted-scoped --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- cat $RO_DIR/test.txt" \
    0

# --- V6-V9 flag smokes ---
run_test "Audit log flags smoke with best-effort" \
    "./landrun --log-level debug --log-disable-originating --log-enable-subprocesses --log-disable-subdomains --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR -- cat $RO_DIR/test.txt" \
    0

run_test "unix on a directory path" \
    "./landrun --log-level debug --rox /usr --ro /lib --ro /lib64 --ro $RO_DIR --unix $RO_DIR -- cat $RO_DIR/test.txt" \
    0

# --- Strict mode (no --best-effort) ---
if [ "$LANDLOCK_ABI" -lt 9 ] 2>/dev/null; then
    run_test_strict "Strict V9 fails on ABI < 9 kernels" \
        "./landrun --log-level error --rox /usr --ro /lib --ro /lib64 -- true" \
        1
else
    print_status "Skipping strict-V9 failure test (kernel ABI >= 9)"
    run_test_strict "Strict V9 succeeds on ABI >= 9 kernels" \
        "./landrun --log-level error --rox /usr --ro /lib --ro /lib64 --add-exec --ldd -- true" \
        0
fi


# Cleanup
print_status "Cleaning up..."
rm -rf "$TEST_DIR"
if [ "$KEEP_BINARY" = false ] && [ "$USE_SYSTEM_BINARY" = false ]; then
    rm -f landrun
fi

print_success "All tests completed!" 
