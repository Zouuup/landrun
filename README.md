# Landrun <img src="https://avatars.githubusercontent.com/u/21111839?s=48&v=4" align="right"/>

A lightweight, secure sandbox for running Linux processes using Landlock. Think firejail, but with kernel-level security and minimal overhead.

Linux Landlock is a kernel-native security module that lets unprivileged processes sandbox themselves.

Landrun is designed to make it practical to sandbox any command with fine-grained filesystem and network access controls. No root. No containers. No SELinux/AppArmor configs.

It's lightweight, auditable, and wraps Landlock up to v11 features (file access, TCP and UDP restrictions, IPC scoping, and UNIX-socket controls).

[Binaries are not trustworthy](https://zoup.org/landrun-binaries-are-not-trustworthy/)

## Features

- 🔒 Kernel-level security using Landlock (up to ABI v11), working on any kernel from ABI v1 onwards
- 🚀 Lightweight and fast execution
- 🛡️ Fine-grained access control for directories and files
- 🔄 Support for read and write paths
- ⚡ Path-specific execution permissions
- 🌐 TCP and UDP network access control (binding and connecting)
- 📡 IPC scoping for abstract UNIX sockets and signals (ABI v6+)
- 🔌 Pathname UNIX domain socket connect/sendmsg control (ABI v9+)
- 📝 Audit logging configuration for Landlock denials (ABI v7+)

## Demo

<p align="center">
  <img src="demo.gif" alt="landrun demo" width="700"/>
</p>

## Requirements

- Linux kernel 5.13 or later with Landlock enabled
- Linux kernel 6.7 or later for TCP network restrictions
- Linux kernel 7.2 or later for UDP network restrictions (Landlock ABI v10)
- Go 1.26 or later (for building from source)

> landrun runs on any Landlock-enabled kernel. It enforces everything the running kernel supports, warns about default restrictions an older kernel cannot enforce, and refuses to run if an option you passed explicitly (e.g. `--connect-udp` on a kernel without UDP support) cannot be enforced. See [Kernel Compatibility](#kernel-compatibility).

## Installation

### Quick Install

```bash
go install github.com/zouuup/landrun/cmd/landrun@latest
```

### Prebuilt Binaries

Each [release](https://github.com/Zouuup/landrun/releases) has statically linked binaries that run on any Linux distribution: `landrun-linux-amd64`, `-arm64`, `-arm` (ARMv7), `-386`, `-riscv64`, `-ppc64le`, `-s390x` and `-loong64`. There is also `landrun-linux-amd64-glibc`, which is dynamically linked and needs glibc 2.35+ (Ubuntu 22.04 or newer); most users want `landrun-linux-amd64`.

Binaries are built by the release workflow in this repository. To verify a download:

```bash
sha256sum --check --ignore-missing SHA256SUMS
gh attestation verify landrun-linux-amd64 --repo Zouuup/landrun
```

The second command checks that the binary was built by this repository's GitHub Actions workflow from the tagged source.

### From Source

```bash
git clone https://github.com/zouuup/landrun.git
cd landrun
CGO_ENABLED=0 go build -trimpath -o landrun ./cmd/landrun
sudo cp landrun /usr/local/bin/
```

### Distros

#### Arch (AUR)

- [stable](https://aur.archlinux.org/packages/landrun) maintained by [Vcalv](https://github.com/vcalv)
- [latest commit](https://aur.archlinux.org/packages/landrun-git) maintained by [juxuanu](https://github.com/juxuanu/)

#### Slackware

maintained by [r1w1s1](https://github.com/r1w1s1)

[Slackbuild](https://slackbuilds.org/repository/15.0/network/landrun/?search=landrun)
```bash
sudo sbopkg -i packagename
```

#### Debian/Ubuntu

Available in Ubuntu since `questing` / `resolute` (26.04 LTS), available in Debian since `forky`.

```bash
sudo apt install landrun
```


## Usage

Basic syntax:

```bash
landrun [options] [--] <command> [args...]
```

Put `--` before the command if it takes arguments starting with `-`, otherwise landrun reads them as its own options.

### Options

- `--ro <path>`: Allow read-only access to specified path (can be specified multiple times or as comma-separated values)
- `--rox <path>`: Allow read-only access with execution to specified path (can be specified multiple times or as comma-separated values)
- `--rw <path>`: Allow read-write access to specified path (can be specified multiple times or as comma-separated values)
- `--rwx <path>`: Allow read-write access with execution to specified path (can be specified multiple times or as comma-separated values)
- `--unix <path>`: Allow `connect(2)`/`sendmsg(2)` on the specified pathname UNIX domain socket, or on every socket under it if `<path>` is a directory; also grants read access to the path (Landlock ABI v9+; ignored with `--unrestricted-filesystem`; can be specified multiple times or as comma-separated values)
- `--bind-tcp <port>`: Allow binding to specified TCP port or port range like `32000-32768` (Landlock ABI v4+; can be specified multiple times or as comma-separated values). Also enforced with `--unrestricted-network`, see below
- `--connect-tcp <port>`: Allow connecting to specified TCP port or port range (Landlock ABI v4+; can be specified multiple times or as comma-separated values)
- `--bind-udp <port>`: Allow binding to specified UDP port or port range (Landlock ABI v10+; can be specified multiple times or as comma-separated values). Also enforced with `--unrestricted-network` (plus ephemeral port 0), see below
- `--connect-udp <port>`: Allow connecting/sending to specified UDP port or port range (Landlock ABI v10+; can be specified multiple times or as comma-separated values). Also allows binding to ephemeral port 0, because the kernel implicitly binds an unbound UDP socket when it first sends
- `--restrict-bind-tcp`: With `--unrestricted-network`, deny TCP binding except to `--bind-tcp` ports, even if none are given (Landlock ABI v4+). Has no effect without `--unrestricted-network`, where binding is already restricted
- `--restrict-bind-udp`: Same as `--restrict-bind-tcp`, for UDP (Landlock ABI v10+). Binding to an ephemeral port (port 0, including the kernel's implicit bind when an unbound socket first sends) stays allowed so outgoing UDP keeps working
- `--env <var>`: Environment variable to pass to the sandboxed command (format: KEY=VALUE or just KEY to pass current value)
- `--best-effort`: Run even if the kernel cannot enforce options you passed explicitly, skipping those restrictions with a `[landrun:warn]` message. Without Landlock, or on ABI v1 with `--rw`/`--rwx` on a directory, the command then runs unsandboxed (also with a warning) [default: disabled]
- `--log-level <level>`: Set logging level (error, info, debug) [default: "error"]
- `--unrestricted-network`: Allow unrestricted network access; `--connect-tcp`/`--connect-udp` are then ignored. If `--bind-tcp`/`--bind-udp` or `--restrict-bind-tcp`/`--restrict-bind-udp` are also given, outgoing access stays unrestricted but binding remains restricted for that protocol
- `--unrestricted-filesystem`: Allows unrestricted filesystem access (disables all filesystem restrictions)
- `--unrestricted-scoped`: Allows unrestricted IPC scoping, i.e. does not restrict abstract UNIX sockets and signals (Landlock ABI v6+) [default: disabled]
- `--ignore-missing`: Gracefully ignore paths that do not exist instead of failing [default: disabled]
- `--log-disable-originating`: Disable audit logging of denials from the originating process (Landlock ABI v7+)
- `--log-enable-subprocesses`: Enable audit logging of denials after `execve(2)` in subprocesses (Landlock ABI v7+)
- `--log-disable-subdomains`: Disable audit logging of denials from nested Landlock domains (Landlock ABI v7+)
- `--add-exec`: Automatically adds the executing binary to --rox
- `--ldd`: Automatically adds required libraries to --rox

### Important Notes

- You must explicitly add the directory or files to the command you want to run with `--rox` flag
- For system commands, you typically need to include `/usr/bin`, `/usr/lib`, and other system directories
- Use `--rwx` for directories or files where you need both write access and the ability to execute files
- TCP network restrictions require Linux kernel 6.7 or later with Landlock ABI v4; UDP network restrictions require Linux kernel 7.2 or later with Landlock ABI v10
- By default, no environment variables are passed to the sandboxed command. Use `--env` to explicitly pass environment variables
- On older kernels, landrun still runs and warns about default restrictions it cannot enforce. It only refuses to run when an option you passed explicitly needs a newer kernel; `--best-effort` skips those options instead, with a warning
- Paths can be specified either using multiple flags or as comma-separated values (e.g., `--ro /usr,/lib,/home`)
- If no paths or network rules are specified and no `--unrestricted-*` flag is set, landrun applies maximum restrictions: it denies all access the running kernel can restrict, and a `[landrun:warn]` message lists any default restriction the kernel cannot enforce
- On Landlock ABI v10+ kernels, UDP (including DNS) is denied unless allowed with `--connect-udp`/`--bind-udp` or `--unrestricted-network`; name resolution usually needs `--connect-udp 53 --ro /etc`
- By default, IPC scoping is restricted: the sandboxed process cannot connect to abstract UNIX sockets or send signals to processes outside its Landlock domain (ABI v6+). Use `--unrestricted-scoped` if this breaks your workload (e.g. some X11 or D-Bus setups)
- On ABI v9+ kernels, connecting to pathname UNIX domain sockets created outside the sandbox (e.g. DNS/NSS via `nscd`, D-Bus, database sockets) is restricted. Grant access to specific sockets with `--unix <path>`

### Environment Variables

- `LANDRUN_LOG_LEVEL`: Set logging level (error, info, debug)

### Examples

1. Run a command that allows exec access to a specific file

```bash
landrun --rox /usr/bin/ls --rox /usr/lib --ro /home ls /home
```

2. Run a command with read-only access to a directory:

```bash
landrun --rox /usr/ --ro /path/to/dir ls /path/to/dir
```

3. Run a command with write access to a directory:

```bash
landrun --rox /usr/bin,/lib,/lib64 --rw /path/to/dir touch /path/to/dir/newfile
```

4. Run a command with write access to a file:

```bash
landrun --rox /usr/bin,/lib,/lib64 --rw /path/to/dir/newfile touch /path/to/dir/newfile
```

5. Run a command with execution permissions:

```bash
landrun --rox /usr/ --ro /lib,/lib64 /usr/bin/bash
```

6. Run with debug logging:

```bash
landrun --log-level debug --rox /usr/ --ro /lib,/lib64,/path/to/dir ls /path/to/dir
```

7. Run with network restrictions:

```bash
landrun --rox /usr/ --ro /lib,/lib64 --bind-tcp 8080 --connect-tcp 80 /usr/bin/my-server
```

This will allow the program to only bind to TCP port 8080 and connect to TCP port 80.

Ports also accept `START-END` ranges, alone or mixed with single ports:

```bash
landrun --rox /usr --bind-tcp 8000-8100 /usr/bin/my-server
landrun --rox /usr --connect-tcp 80,443,32000-32768 /usr/bin/my-client
landrun --best-effort --rox /usr --bind-udp 5000-5010 --connect-udp 53 /usr/bin/my-udp-server
```

8. Run with unrestricted outgoing network access while restricting bind ports:

```bash
landrun --rox /usr/ --ro /lib,/lib64 --unrestricted-network --bind-tcp 8080 --bind-udp 5353 /usr/bin/my-server
```

This permits all outgoing connections while only allowing the program to bind TCP port 8080 and UDP port 5353 (plus ephemeral UDP ports for outgoing traffic). Use `--restrict-bind-tcp` to deny TCP binding entirely, or `--restrict-bind-udp` to deny UDP binding to any fixed port (ephemeral ports stay allowed so outgoing UDP works). UDP bind restrictions need Landlock ABI v10: on older kernels this command refuses to run unless you add `--best-effort`, which skips them.

9. Run a DNS client with appropriate permissions:

```bash
landrun --log-level debug --ro /etc,/usr --rox /usr/ --connect-tcp 443 --connect-udp 53 nc kernel.org 443
```

This allows connections to port 443 and DNS queries to UDP port 53, and requires access to /etc/resolv.conf for resolving DNS. On kernels older than Landlock ABI v10, add `--best-effort`. If the host resolves names through systemd-resolved or nscd sockets, also allow those with `--unix`.

10. Run a web server with selective network permissions:

```bash
landrun --rox /usr/bin,/lib,/lib64 --ro /var/www --rwx /var/log --bind-tcp 80,443 /usr/bin/nginx
```

11. Running anything without providing parameters is... maximum security jail!

```bash
landrun ls
```

12. If you keep getting permission denied without knowing what exactly going on, best to use strace with it.

```bash
landrun --rox /usr -- strace -f -e trace=all /usr/bin/ls
```

13. Run with specific environment variables:

```bash
landrun --rox /usr --ro /etc --env HOME --env PATH --env CUSTOM_VAR=my_value -- env
```

This example passes the current HOME and PATH variables, plus a custom variable named CUSTOM_VAR.

14. Run command with explicity access to files instead of directories:
```bash
landrun --rox /usr/lib/libc.so.6 --rox /usr/lib64/ld-linux-x86-64.so.2  --rox /usr/bin/true /usr/bin/true
```

15. Run a command with --add-exec which automatically adds target binary to --rox

```bash
landrun --rox /usr/lib/ --add-exec /usr/bin/true
```

16. Run a command with --ldd and --add-exec which automatically adds required libraries and target binary to --rox

```bash
landrun --ldd --add-exec /usr/bin/true
```

Note that shared libs always need exec permission due to how they are loaded, PROT_EXEC on mmap() etc.

17. Allow connecting to a pathname UNIX domain socket (ABI v9+), e.g. a database socket:

```bash
landrun --rox /usr --ro /etc --unix /run/postgresql/.s.PGSQL.5432 -- psql ...
```

On kernels older than Landlock ABI v9, landrun refuses to run this; add `--best-effort` to run without the socket restriction.

18. Gracefully ignore optional paths that may not exist:

```bash
landrun --ignore-missing --rox /usr --ro /etc,/opt/optional-config -- myapp
```

19. Allow abstract UNIX sockets / signals to reach outside the sandbox (relax IPC scoping):

```bash
landrun --unrestricted-scoped --rox /usr --ro /etc -- some-gui-app
```

## Systemd Integration

landrun can be integrated with systemd to run services with enhanced security. Here's an example of running nginx with landrun:

1. Create a systemd service file (e.g., `/etc/systemd/system/nginx-landrun.service`):

```ini
[Unit]
Description=nginx with landrun sandbox
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/landrun \
    --rox /usr/bin,/usr/lib \
    --ro  /etc/nginx,/etc/ssl,/etc/passwd,/etc/group,/etc/nsswitch.conf \
    --rwx /var/log/nginx \
    --rwx /var/cache/nginx \
    --bind-tcp 80,443 \
    /usr/bin/nginx -g 'daemon off;'
Restart=always
User=nginx
Group=nginx

[Install]
WantedBy=multi-user.target
```

2. Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable nginx-landrun
sudo systemctl start nginx-landrun
```

3. Check the service status:

```bash
sudo systemctl status nginx-landrun
```

This configuration:
- Runs nginx with minimal required permissions
- Allows binding to ports 80 and 443
- Provides read-only access to configuration files
- Allows write access only to log and cache directories
- Runs as the nginx user and group
- Automatically restarts on failure

You can adjust the permissions based on your specific needs. For example, if you need to serve static files from `/var/www`, add `--ro /var/www` to the ExecStart line.

## Security

landrun uses Linux's Landlock to create a secure sandbox environment. It provides:

- File system access control
- Directory access restrictions
- Execution control
- TCP and UDP network restrictions
- Pathname UNIX domain socket connect/sendmsg restrictions (ABI v9+; allow specific sockets with `--unix`)
- IPC scoping of abstract UNIX sockets and signals (ABI v6+; relax with `--unrestricted-scoped`)
- Default restrictive mode when no rules are specified

Landlock is an access-control system that enables processes to securely restrict themselves and their future children. As a stackable Linux Security Module (LSM), it creates additional security layers on top of existing system-wide access controls, helping to mitigate security impacts from bugs or malicious behavior in applications.

### Landlock Access Control Rights

landrun leverages Landlock's fine-grained access control mechanisms, which include:

**File-specific rights:**

- Execute files (`LANDLOCK_ACCESS_FS_EXECUTE`)
- Write to files (`LANDLOCK_ACCESS_FS_WRITE_FILE`)
- Read files (`LANDLOCK_ACCESS_FS_READ_FILE`)
- Truncate files (`LANDLOCK_ACCESS_FS_TRUNCATE`) - Available since Landlock ABI v3
- IOCTL operations on devices (`LANDLOCK_ACCESS_FS_IOCTL_DEV`) - Available since Landlock ABI v5
- Connect/sendmsg on pathname UNIX sockets (`LANDLOCK_ACCESS_FS_RESOLVE_UNIX`) - Available since Landlock ABI v9 (granted per-path with `--unix`)

**Directory-specific rights:**

- Read directory contents (`LANDLOCK_ACCESS_FS_READ_DIR`)
- Remove directories (`LANDLOCK_ACCESS_FS_REMOVE_DIR`)
- Remove files (`LANDLOCK_ACCESS_FS_REMOVE_FILE`)
- Create various filesystem objects (char devices, directories, regular files, sockets, etc.)
- Refer/reparent files across directories (`LANDLOCK_ACCESS_FS_REFER`) - Available since Landlock ABI v2

**Network-specific rights** (TCP requires Linux 6.7+ with Landlock ABI v4; UDP requires Linux 7.2+ with Landlock ABI v10):

- Bind to specific TCP ports (`LANDLOCK_ACCESS_NET_BIND_TCP`)
- Connect to specific TCP ports (`LANDLOCK_ACCESS_NET_CONNECT_TCP`)
- Bind to specific UDP ports (`LANDLOCK_ACCESS_NET_BIND_UDP`)
- Connect to and send datagrams to specific UDP ports (`LANDLOCK_ACCESS_NET_CONNECT_SEND_UDP`)

**IPC scoping** (requires Linux 6.12+ with Landlock ABI v6):

- Restrict connections to abstract UNIX sockets outside the domain (`LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET`)
- Restrict sending signals to processes outside the domain (`LANDLOCK_SCOPE_SIGNAL`)

These are restricted by default and can be relaxed with `--unrestricted-scoped`.

### Limitations

- Landlock must be supported by your kernel
- TCP network restrictions require Linux kernel 6.7 or later with Landlock ABI v4; UDP network restrictions require Linux kernel 7.2 or later with Landlock ABI v10
- TCP restrictions only apply to "classic" TCP sockets, not Multipath TCP. Since Go 1.24, `net.Listen` defaults to Multipath TCP and therefore cannot currently be restricted by Landlock (kernel bug [landlock-lsm/linux#54](https://github.com/landlock-lsm/linux/issues/54))
- Some operations may require additional permissions
- Files or directories opened before sandboxing are not subject to Landlock restrictions
- setuid/setgid binaries and file capabilities cannot gain privileges inside landrun (e.g. `sudo`, `pkexec`, `su`), because Landlock requires the `no_new_privs` flag. Gain privileges before sandboxing instead, e.g. `sudo landrun ...`

## Kernel Compatibility Table

| Feature                                       | Minimum Kernel Version | Landlock ABI Version |
| --------------------------------------------- | ---------------------- | -------------------- |
| Basic filesystem sandboxing                   | 5.13                   | 1                    |
| File referring/reparenting control            | 5.19                   | 2                    |
| File truncation control                       | 6.2                    | 3                    |
| Network TCP restrictions                      | 6.7                    | 4                    |
| IOCTL on special files                        | 6.10                   | 5                    |
| IPC scoping (abstract UNIX sockets, signals)  | 6.12                   | 6                    |
| Audit logging of denials                      | 6.15                   | 7                    |
| Thread synchronization (TSYNC)                | 7.0                    | 8                    |
| Pathname UNIX socket connect/sendmsg control  | latest                 | 9                    |
| UDP network restrictions (bind/connect/send)  | 7.2                    | 10                   |
| Atomic `no_new_privs` on enforcement          | latest                 | 11                   |

## Troubleshooting

If you receive "permission denied" or similar errors:

1. Ensure you've added all necessary paths with `--ro`/`--rw`, and the executable and its libraries with `--rox`/`--rwx` (or use `--add-exec`/`--ldd` to add them automatically)
2. Try running with `--log-level debug` to see detailed permission information
3. Check that Landlock is supported and enabled on your system:
   ```bash
   grep -E 'landlock|lsm=' /boot/config-$(uname -r)
   # alternatively, if there are no /boot/config-* files
   zgrep -iE 'landlock|lsm=' /proc/config.gz
   # another alternate method
   grep -iE 'landlock|lsm=' /lib/modules/$(uname -r)/config
   ```
   You should see `CONFIG_SECURITY_LANDLOCK=y` and `lsm=landlock,...` in the output
4. Check which Landlock ABI landrun actually uses (`uname -r` is not enough, see [Kernel Compatibility](#kernel-compatibility)):
   ```bash
   landrun --log-level debug --rox /usr -- true 2>&1 | grep 'Detected Landlock ABI'
   ```
   TCP rules need ABI v4 (Linux 6.7+); UDP rules need ABI v10 (Linux 7.2+).
5. An error like `kernel supports Landlock ABI vN, but <option> needs ABI vM` means you asked for a restriction the kernel cannot enforce. Upgrade the kernel, drop that option, or add `--best-effort` to run without it.
6. A `[landrun:warn] ... default restrictions are not enforced: ...` message is not an error: the command still runs, but those restrictions are not applied on this kernel.

## Technical Details

### Implementation

This project uses the [landlock-lsm/go-landlock](https://github.com/landlock-lsm/go-landlock) package (v0.11.2) for sandboxing, which provides filesystem, network and IPC-scope restrictions. The current implementation targets Landlock ABI v11 and supports:

- Read/write/execute restrictions for files and directories
- TCP and UDP port binding restrictions
- TCP and UDP port connection restrictions
- IPC scoping (abstract UNIX sockets and signals)
- Pathname UNIX domain socket connect/sendmsg control (`--unix`)
- Audit logging configuration for Landlock denials (`--log-*`)
- Graceful handling of missing paths (`--ignore-missing`)
- Graceful degradation on older kernels (see [Kernel Compatibility](#kernel-compatibility))

### Kernel Compatibility

landrun targets the newest Landlock ABI it knows (v11) and works on every kernel from ABI v1 onwards, as well as on future kernels with a newer ABI.

landrun separates what you **explicitly asked for** from the **default restrictions** it applies to everything else:

- **Explicit options** have a minimum ABI. If the kernel is older, landrun refuses to run and says which option needs which ABI, so a restriction you asked for is never silently dropped:

  | Option | Minimum Landlock ABI |
  |---|---|
  | any filesystem restriction | v1 |
  | `--rw`/`--rwx` on a directory (rename/link across directories) | v2 |
  | `--bind-tcp`, `--connect-tcp`, `--restrict-bind-tcp` | v4 |
  | `--log-disable-originating`, `--log-enable-subprocesses`, `--log-disable-subdomains` | v7 |
  | `--unix` | v9 |
  | `--bind-udp`, `--connect-udp`, `--restrict-bind-udp` | v10 |

  The table only applies to options that take effect: `--restrict-bind-*` and the `--bind-*` restrictions under `--unrestricted-network` count, but `--connect-*` is ignored with `--unrestricted-network` and `--unix` with `--unrestricted-filesystem`.

- **Default restrictions**, such as denying UDP when you passed no UDP options, are enforced as far as the kernel supports them. Anything the kernel cannot enforce is listed in a `[landrun:warn]` message on stderr. Warnings are printed at every `--log-level`, and the command still runs.

With `--best-effort`, landrun also runs when an explicit option needs a newer kernel. It skips that restriction and says so in a `[landrun:warn]` message. On ABI v1, `--rw`/`--rwx` on a directory disables the whole sandbox, because v1 cannot grant renames across directories; without Landlock, the command runs unsandboxed. Both are reported as warnings too.

landrun uses the ABI that go-landlock actually enforces, which is not always what the kernel reports: a kernel reporting ABI v6 or newer without the signal-scoping fix (Landlock erratum 2) is treated as ABI v5. Run with `--log-level debug` to see the detected ABI.

What each kernel can enforce:

- ABI v11: Same restrictions as v10; go-landlock sets `no_new_privs` atomically with the sandbox
- On Linux 7.2+: All of the below, plus UDP network restrictions (ABI v10)
- On Linux 7.0+: All of the below, plus thread synchronization (TSYNC) and, on ABI v9 kernels, pathname UNIX socket controls
- On Linux 6.15+: Adds audit logging configuration (ABI v7)
- On Linux 6.12+: Adds IPC scoping for abstract UNIX sockets and signals (ABI v6)
- On Linux 6.10+: Filesystem, network and IOCTL restrictions (ABI v5)
- On Linux 6.7+: Full filesystem and network restrictions (ABI v4)
- On Linux 6.2-6.6: Filesystem restrictions including truncation, but no network restrictions
- On Linux 5.19-6.1: Basic filesystem restrictions including file reparenting, but no truncation control or network restrictions
- On Linux 5.13-5.18: Basic filesystem restrictions without file reparenting, truncation control, or network restrictions
- Without Landlock: landrun refuses to run (filesystem restrictions need ABI v1), unless `--best-effort` is given, which runs the command unsandboxed with a warning

When no rules are specified and no `--unrestricted-*` flag is set, landrun applies the maximum restrictions available for the current kernel version.

### Tests

The project includes a comprehensive test suite that verifies:

- Basic filesystem access controls (read-only, read-write, execute)
- Directory traversal and path handling
- Network restrictions (TCP and UDP bind/connect, port ranges, bind-only restrictions with `--unrestricted-network`)
- Pathname UNIX socket access (`--unix`) and audit-logging flags
- Strict mode vs `--best-effort`, depending on the detected Landlock ABI
- Environment variable isolation
- System command execution
- Edge cases and regression tests

Run the tests with:

```bash
./test.sh
```

Use `--keep-binary` to preserve the test binary after completion:

```bash
./test.sh --keep-binary
```

Use `--use-system` to test against the system-installed landrun binary:

```bash
./test.sh --use-system
```

Use `--no-build` to reuse an existing `./landrun` binary, and `--offline` to skip tests that need internet access. ABI-dependent tests are skipped if the Landlock ABI cannot be detected (detection needs the Go toolchain).

The Go packages also have unit tests:

```bash
go test ./...
```

## Future Features

Based on the Linux Landlock API capabilities, we plan to add:

- 🔒 Enhanced filesystem controls with more fine-grained permissions
- 🌐 Support for additional network protocol restrictions as Landlock introduces them
- 🛡️ Additional security features as they become available in the Landlock API

## Acknowledgements

This project wouldn't exist without:

- [Landlock](https://landlock.io), the kernel security module enabling unprivileged sandboxing - maintained by [@l0kod](https://github.com/l0kod)
- [go-landlock](https://github.com/landlock-lsm/go-landlock), the Go bindings powering this tool - developed by [@gnoack](https://github.com/gnoack)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
