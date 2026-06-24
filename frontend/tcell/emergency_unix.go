//go:build unix

package tcell

/*
#include <signal.h>
#include <string.h>
#include <termios.h>
#include <unistd.h>

static struct termios emergency_orig;
static int emergency_saved;

static void emergency_write(const char *s) {
	size_t n = strlen(s);
	if (n > 0) {
		(void)write(STDERR_FILENO, s, n);
	}
}

static const char *emergency_sig_name(int sig) {
	switch (sig) {
	case SIGSEGV:
		return "Segmentation fault";
	case SIGBUS:
		return "Bus error";
	case SIGFPE:
		return "Floating-point exception";
	case SIGILL:
		return "Illegal instruction";
#ifdef SIGABRT
	case SIGABRT:
		return "Abort";
#endif
#ifdef SIGSYS
	case SIGSYS:
		return "Bad system call";
#endif
#ifdef SIGXCPU
	case SIGXCPU:
		return "Cputime limit exceeded";
#endif
#ifdef SIGXFSZ
	case SIGXFSZ:
		return "Filesize limit exceeded";
#endif
	default:
		return "Signal";
	}
}

static void emergency_cleanup_screen(void) {
	// Best-effort alternate-screen and cursor restore (async-signal-safe).
	emergency_write("\033[?1049l\033[?25h\033[0m\r\n");
}

static void emergency_handler(int sig) {
	if (emergency_saved) {
		(void)tcsetattr(STDIN_FILENO, TCSADRAIN | TCSASOFT, &emergency_orig);
	}
	emergency_cleanup_screen();
	emergency_write(emergency_sig_name(sig));
	emergency_write("\n");
	_exit(128 + sig);
}

static void emergency_install_one(int sig) {
	struct sigaction act;
	memset(&act, 0, sizeof act);
	act.sa_handler = emergency_handler;
	sigemptyset(&act.sa_mask);
	(void)sigaction(sig, &act, NULL);
}

static void emergency_install_handlers(void) {
	emergency_install_one(SIGSEGV);
	emergency_install_one(SIGBUS);
	emergency_install_one(SIGFPE);
	emergency_install_one(SIGILL);
#ifdef SIGABRT
	emergency_install_one(SIGABRT);
#endif
#ifdef SIGSYS
	emergency_install_one(SIGSYS);
#endif
#ifdef SIGXCPU
	emergency_install_one(SIGXCPU);
#endif
#ifdef SIGXFSZ
	emergency_install_one(SIGXFSZ);
#endif
}

static void emergency_save_termios(void) {
	if (tcgetattr(STDIN_FILENO, &emergency_orig) == 0) {
		emergency_saved = 1;
	}
}
*/
import "C"

func saveEmergencyTermios() { C.emergency_save_termios() }

func installEmergencyHandlers() { C.emergency_install_handlers() }
