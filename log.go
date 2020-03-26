package gou

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Logging levels
const (
	NOLOGGING = -1
	FATAL     = 0
	ERROR     = 1
	WARN      = 2
	INFO      = 3
	DEBUG     = 4
)

/*
https://github.com/mewkiz/pkg/tree/master/term
RED = '\033[0;1;31m'
GREEN = '\033[0;1;32m'
YELLOW = '\033[0;1;33m'
BLUE = '\033[0;1;34m'
MAGENTA = '\033[0;1;35m'
CYAN = '\033[0;1;36m'
WHITE = '\033[0;1;37m'
DARK_MAGENTA = '\033[0;35m'
ANSI_RESET = '\033[0m'
LogColor         = map[int]string{FATAL: "\033[0m\033[37m",
	ERROR: "\033[0m\033[31m",
	WARN:  "\033[0m\033[33m",
	INFO:  "\033[0m\033[32m",
	DEBUG: "\033[0m\033[34m"}

\e]PFdedede
*/

var (
	// LogLevel current log level
	LogLevel int = ERROR
	// ErrLogLevel is the Error log level
	ErrLogLevel int = ERROR
	// LogColor maps log levels to terminal colors
	LogColor = map[int]string{FATAL: "\033[0m\033[37m",
		ERROR: "\033[0m\033[31m",
		WARN:  "\033[0m\033[33m",
		INFO:  "\033[0m\033[35m",
		DEBUG: "\033[0m\033[34m"}
	//LogPrefix maps log levels to prefixs
	LogPrefix = map[int]string{
		FATAL: "[FATAL] ",
		ERROR: "[ERROR] ",
		WARN:  "[WARN] ",
		INFO:  "[INFO] ",
		DEBUG: "[DEBUG] ",
	}
	// LogLevelWords maps log level strings to loglevels
	LogLevelWords  = map[string]int{"fatal": 0, "error": 1, "warn": 2, "info": 3, "debug": 4, "none": -1}
	logger         *log.Logger
	customLogger   LoggerCustom
	loggerErr      *log.Logger
	escapeNewlines = false
	postFix        = "" //\033[0m
	logThrottles   = make(map[string]*Throttler)
	throttleMu     sync.Mutex
)

const (
	// logContextPrefixKey is the context key used to store log prefixes
	logContextPrefixKey logContextType = 1
)

// logContextType is the type used for logger context keys to prevent collisions with other context keys
type logContextType int

// LoggerCustom defines custom interface for logger implementation
type LoggerCustom interface {
	Log(depth, logLevel int, msg string, fields map[string]interface{})
}

// SetupLogging sets up default logging to Stderr, equivalent to:
//
//	gou.SetLogger(log.New(os.Stderr, "", log.Ltime|log.Lshortfile), "debug")
func SetupLogging(lvl string) {
	SetLogger(log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile|log.Lmicroseconds), strings.ToLower(lvl))
}

// SetupLoggingLong sets up default logging to Stderr, equivalent to:
//
//	gou.SetLogger(log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile|log.Lmicroseconds), level)
func SetupLoggingLong(lvl string) {
	SetLogger(log.New(os.Stderr, "", log.LstdFlags|log.Llongfile|log.Lmicroseconds), strings.ToLower(lvl))
}

// SetupLoggingFile writes logs to the file object parameter.
func SetupLoggingFile(f *os.File, lvl string) {
	SetLogger(log.New(f, "", log.LstdFlags|log.Lshortfile|log.Lmicroseconds), strings.ToLower(lvl))
}

// SetCustomLogger sets the logger to a custom logger
func SetCustomLogger(cl LoggerCustom) {
	customLogger = cl
}

// GetCustomLogger returns the custom logger if initialized
func GetCustomLogger() LoggerCustom {
	return customLogger
}

// SetColorIfTerminal sets up colorized output if this is a terminal
func SetColorIfTerminal() {
	if IsTerminal() {
		SetColorOutput()
	}
}

// SetColorOutput sets up colorized output
func SetColorOutput() {
	for lvl, color := range LogColor {
		LogPrefix[lvl] = color
	}
	postFix = "\033[0m"
}

// SetEscapeNewlines sets whether to escape newline characters in log messages
func SetEscapeNewlines(en bool) {
	escapeNewlines = en
}

// DiscardStandardLogger sets up default log output to go to a dev/null
//
//	log.SetOutput(new(DevNull))
func DiscardStandardLogger() {
	log.SetOutput(new(DevNull))
}

// SetLogger lets you can set a logger, and log level,most common usage is:
//
//	gou.SetLogger(log.New(os.Stdout, "", log.LstdFlags), "debug")
//
//  loglevls:   debug, info, warn, error, fatal
// Note, that you can also set a separate Error Log Level
func SetLogger(l *log.Logger, logLevel string) {
	logger = l
	LogLevelSet(logLevel)
}

// GetLogger gets the logger set by SetLogger
func GetLogger() *log.Logger {
	return logger
}

// SetErrLogger lets you set a logger, and log level for errors, and assumes
// you are logging to Stderr (seperate from stdout above), allowing you to seperate
// debug&info logging from errors
//
//	gou.SetLogger(log.New(os.Stderr, "", log.LstdFlags), "debug")
//
//  loglevls:   debug, info, warn, error, fatal
func SetErrLogger(l *log.Logger, logLevel string) {
	loggerErr = l
	if lvl, ok := LogLevelWords[logLevel]; ok {
		ErrLogLevel = lvl
	}
}

// GetErrLogger gets the logger set by SetErrLogger
func GetErrLogger() *log.Logger {
	return logger
}

// LogLevelSet sets the log level from a string
func LogLevelSet(levelWord string) {
	if lvl, ok := LogLevelWords[levelWord]; ok {
		LogLevel = lvl
	}
}

// NewContext returns a new Context carrying contextual log message
// that gets prefixed to log statements.
func NewContext(ctx context.Context, msg string) context.Context {
	return context.WithValue(ctx, logContextPrefixKey, msg)
}

// NewContextWrap returns a new Context carrying contextual log message
// that gets prefixed to log statements.
func NewContextWrap(ctx context.Context, msg string) context.Context {
	logContext, ok := ctx.Value(logContextPrefixKey).(string)
	if ok {
		return context.WithValue(ctx, logContextPrefixKey, fmt.Sprintf("%s %s", logContext, msg))
	}
	return context.WithValue(ctx, logContextPrefixKey, msg)
}

// FromContext extracts the Log Context prefix from context
func FromContext(ctx context.Context) string {
	logContext, _ := ctx.Value(logContextPrefixKey).(string)
	return logContext
}

// Debug logs at debug level
func Debug(v ...interface{}) {
	if LogLevel >= 4 {
		DoLog(3, DEBUG, fmt.Sprint(v...))
	}
}

// Debugf logs at debug level with a formatted log
func Debugf(format string, v ...interface{}) {
	if LogLevel >= 4 {
		DoLog(3, DEBUG, fmt.Sprintf(format, v...))
	}
}

// DebugCtx logs at debug level with a formatted context writer
func DebugCtx(ctx context.Context, format string, v ...interface{}) {
	if LogLevel >= 4 {
		lc := FromContext(ctx)
		if len(lc) > 0 {
			format = fmt.Sprintf("%s %s", lc, format)
		}
		DoLog(3, DEBUG, fmt.Sprintf(format, v...))
	}
}

// DebugT logs a trace at debug level
func DebugT(lineCt int) {
	if LogLevel >= 4 {
		DoLog(3, DEBUG, fmt.Sprint("\n", PrettyStack(lineCt)))
	}
}

// Info logs at info level
func Info(v ...interface{}) {
	if LogLevel >= 3 {
		DoLog(3, INFO, fmt.Sprint(v...))
	}
}

// Infof logs at info level with a formatted line
func Infof(format string, v ...interface{}) {
	if LogLevel >= 3 {
		DoLog(3, INFO, fmt.Sprintf(format, v...))
	}
}

// InfoCtx logs at info level with a formatted context writer
func InfoCtx(ctx context.Context, format string, v ...interface{}) {
	if LogLevel >= 3 {
		lc := FromContext(ctx)
		if len(lc) > 0 {
			format = fmt.Sprintf("%s %s", lc, format)
		}
		DoLog(3, INFO, fmt.Sprintf(format, v...))
	}
}

// InfoT logs a trace at info level
func InfoT(lineCt int) {
	if LogLevel >= 3 {
		DoLog(3, INFO, fmt.Sprint("\n", PrettyStack(lineCt)))
	}
}

// Warn logs at warn level
func Warn(v ...interface{}) {
	if LogLevel >= 2 {
		DoLog(3, WARN, fmt.Sprint(v...))
	}
}

// Warnf logs a formatted log at warn level
func Warnf(format string, v ...interface{}) {
	if LogLevel >= 2 {
		DoLog(3, WARN, fmt.Sprintf(format, v...))
	}
}

// WarnCtx logs at warn level with a formatted context writer
func WarnCtx(ctx context.Context, format string, v ...interface{}) {
	if LogLevel >= 2 {
		lc := FromContext(ctx)
		if len(lc) > 0 {
			format = fmt.Sprintf("%s %s", lc, format)
		}
		DoLog(3, WARN, fmt.Sprintf(format, v...))
	}
}

// WarnT logs a trace at warn level
func WarnT(lineCt int) {
	if LogLevel >= 2 {
		DoLog(3, WARN, fmt.Sprint("\n", PrettyStack(lineCt)))
	}
}

// Error logs at error level
func Error(v ...interface{}) {
	if LogLevel >= 1 {
		DoLog(3, ERROR, fmt.Sprint(v...))
	}
}

// Errorf logs a formatted log at error level
func Errorf(format string, v ...interface{}) {
	if LogLevel >= 1 {
		DoLog(3, ERROR, fmt.Sprintf(format, v...))
	}
}

// ErrorCtx logs at error level with a formatted context writer
func ErrorCtx(ctx context.Context, format string, v ...interface{}) {
	if LogLevel >= 1 {
		lc := FromContext(ctx)
		if len(lc) > 0 {
			format = fmt.Sprintf("%s %s", lc, format)
		}
		DoLog(3, ERROR, fmt.Sprintf(format, v...))
	}
}

// LogErrorf logs this error at error level, and returns an error object
func LogErrorf(format string, v ...interface{}) error {
	err := fmt.Errorf(format, v...)
	if LogLevel >= 1 {
		DoLog(3, ERROR, err.Error())
	}
	return err
}

// Log to logger if setup
//
//    Log(ERROR, "message")
func Log(logLvl int, v ...interface{}) {
	if LogLevel >= logLvl {
		DoLog(3, logLvl, fmt.Sprint(v...))
	}
}

// LogTracef logs to logger if setup, grab a stack trace and add that as well
//
//    u.LogTracef(u.ERROR, "message %s", varx)
//
func LogTracef(logLvl int, format string, v ...interface{}) {
	if LogLevel >= logLvl {
		// grab a stack trace
		stackBuf := make([]byte, 6000)
		stackBufLen := runtime.Stack(stackBuf, false)
		stackTraceStr := string(stackBuf[0:stackBufLen])
		parts := strings.Split(stackTraceStr, "\n")
		if len(parts) > 1 {
			v = append(v, strings.Join(parts[3:], "\n"))
		}
		DoLog(3, logLvl, fmt.Sprintf(format+"\n%v", v...))
	}
}

// LogTraceDf logs to logger if setup, grab a stack trace and add that as well,
// limits stack trace to @lineCt number of lines
//
//    u.LogTraceDf(u.ERROR, "message %s", varx)
//
func LogTraceDf(logLvl, lineCt int, format string, v ...interface{}) {
	if LogLevel >= logLvl {
		// grab a stack trace
		stackBuf := make([]byte, 6000)
		stackBufLen := runtime.Stack(stackBuf, false)
		stackTraceStr := string(stackBuf[0:stackBufLen])
		parts := strings.Split(stackTraceStr, "\n")
		if len(parts) > 1 {
			if (len(parts) - 3) > lineCt {
				parts = parts[3 : 3+lineCt]
				parts2 := make([]string, 0, len(parts)/2)
				for i := 1; i < len(parts); i = i + 2 {
					parts2 = append(parts2, parts[i])
				}
				v = append(v, strings.Join(parts2, "\n"))
				//v = append(v, strings.Join(parts[3:3+lineCt], "\n"))
			} else {
				v = append(v, strings.Join(parts[3:], "\n"))
			}
		}
		DoLog(3, logLvl, fmt.Sprintf(format+"\n%v", v...))
	}
}

// PrettyStack is a helper to pull stack-trace, prettify it.
func PrettyStack(lineCt int) string {
	stackBuf := make([]byte, 10000)
	stackBufLen := runtime.Stack(stackBuf, false)
	stackTraceStr := string(stackBuf[0:stackBufLen])
	parts := strings.Split(stackTraceStr, "\n")
	if len(parts) > 3 {
		parts = parts[2:]
		parts2 := make([]string, 0, len(parts)/2)
		for i := 3; i < len(parts)-1; i++ {
			if !strings.HasSuffix(parts[i], ")") && !strings.HasPrefix(parts[i], "/usr/local") {
				parts2 = append(parts2, parts[i])
			}
		}
		if len(parts2) > lineCt {
			return strings.Join(parts2[0:lineCt], "\n")
		}
		return strings.Join(parts2, "\n")
	}
	return stackTraceStr
}

// LogThrottleKey throttles logging based on key, such that key would never occur more than
// @limit times per hour
//
//    LogThrottleKey(u.ERROR, 1,"error_that_happens_a_lot" "message %s", varx)
//
func LogThrottleKey(logLvl, limit int, key, format string, v ...interface{}) {
	if LogLevel < logLvl {
		return
	}

	doLogThrottle(4, logLvl, limit, key, fmt.Sprintf(format, v...))
}

// LogThrottleKeyCtx throttled log formatted context writer. Throttles logging based on @format as a key,
// such that key would never occur more than @limit times per hour
func LogThrottleKeyCtx(ctx context.Context, logLvl, limit int, key, format string, v ...interface{}) {
	if LogLevel < logLvl {
		return
	}

	lc := FromContext(ctx)
	if len(lc) > 0 {
		format = fmt.Sprintf("%s %s", lc, format)
	}
	doLogThrottle(4, logLvl, limit, key, fmt.Sprintf(format, v...))
}

// LogThrottle logging based on @format as a key, such that key would never occur more than
// @limit times per hour
//
//    LogThrottle(u.ERROR, 1, "message %s", varx)
//
func LogThrottle(logLvl, limit int, format string, v ...interface{}) {
	if LogLevel < logLvl {
		return
	}
	doLogThrottle(4, logLvl, limit, format, fmt.Sprintf(format, v...))
}

// LogThrottleCtx log formatted context writer, limits to @limit/hour.
func LogThrottleCtx(ctx context.Context, logLvl, limit int, format string, v ...interface{}) {
	if LogLevel < logLvl {
		return
	}

	lc := FromContext(ctx)
	if len(lc) > 0 {
		format = fmt.Sprintf("%s %s", lc, format)
	}
	doLogThrottle(4, logLvl, limit, format, fmt.Sprintf(format, v...))
}

// LogThrottleD Throttle logging based on @format as a key, such that key would never
// occur more than @limit times per hour
//
//    LogThrottleD(5, u.ERROR, 1, "message %s", varx)
//
func LogThrottleD(depth, logLvl, limit int, format string, v ...interface{}) {
	if LogLevel < logLvl {
		return
	}
	doLogThrottle(depth, logLvl, limit, format, fmt.Sprintf(format, v...))
}

func doLogThrottle(depth, logLvl, limit int, key, msg string) {
	if LogLevel < logLvl {
		return
	}
	throttleMu.Lock()
	th, ok := logThrottles[key]
	if !ok {
		th = NewThrottler(limit, 3600*time.Second)
		logThrottles[key] = th
	}
	skip, _ := th.Throttle()
	throttleMu.Unlock()
	if skip {
		return
	}

	DoLog(depth, logLvl, msg)
}

// Logf to logger if setup
//    Logf(ERROR, "message %d", 20)
func Logf(logLvl int, format string, v ...interface{}) {
	if LogLevel >= logLvl {
		DoLog(3, logLvl, fmt.Sprintf(format, v...))
	}
}

// LogFieldsf logs to the logger,it including additional context fields if a custom logger is setup
func LogFieldsf(logLvl int, fields map[string]interface{}, format string, v ...interface{}) {
	if LogLevel >= logLvl {
		DoLogFields(3, logLvl, fmt.Sprintf(format, v...), fields)
	}
}

// LogP logs to logger if setup
//    LogP(ERROR, "prefix", "message", anyItems, youWant)
func LogP(logLvl int, prefix string, v ...interface{}) {
	if ErrLogLevel >= logLvl && loggerErr != nil {
		loggerErr.Output(3, prefix+LogPrefix[logLvl]+fmt.Sprint(v...)+postFix)
	} else if LogLevel >= logLvl && logger != nil {
		logger.Output(3, prefix+LogPrefix[logLvl]+fmt.Sprint(v...)+postFix)
	}
}

// LogPf logs to logger if setup with a prefix
//    LogPf(ERROR, "prefix", "formatString %s %v", anyItems, youWant)
func LogPf(logLvl int, prefix string, format string, v ...interface{}) {
	if ErrLogLevel >= logLvl && loggerErr != nil {
		loggerErr.Output(3, prefix+LogPrefix[logLvl]+fmt.Sprintf(format, v...)+postFix)
	} else if LogLevel >= logLvl && logger != nil {
		logger.Output(3, prefix+LogPrefix[logLvl]+fmt.Sprintf(format, v...)+postFix)
	}
}

// LogD lets you log with a modified stackdepth
// When you want to use the log short filename flag, and want to use
// the lower level logging functions (say from an *Assert* type function)
// you need to modify the stack depth:
//
//     func init() {}
// 	       SetLogger(log.New(os.Stderr, "", log.Ltime|log.Lshortfile|log.Lmicroseconds), lvl)
//     }
//
//     func assert(t *testing.T, myData) {
//         // we want log line to show line that called this assert, not this line
//         LogD(5, DEBUG, v...)
//     }
func LogD(depth int, logLvl int, v ...interface{}) {
	if LogLevel >= logLvl {
		DoLog(depth, logLvl, fmt.Sprint(v...))
	}
}

// DoLog is a low level log with depth , level, message and logger
func DoLog(depth, logLvl int, msg string) {
	DoLogFields(depth+1, logLvl, msg, nil)
}

// LogCtx Low level log with depth , level, message and logger
func LogCtx(ctx context.Context, depth, logLvl int, format string, v ...interface{}) {
	if LogLevel >= logLvl {
		msg := fmt.Sprintf(format, v...)
		lc := FromContext(ctx)
		if len(lc) > 0 {
			msg = fmt.Sprintf("%s %s", lc, msg)
		}
		DoLog(depth+1, logLvl, msg)
	}
}

// DoLogFields allows the inclusion of additional context for logrus logs
// file and line number are included in the fields by default
func DoLogFields(depth, logLvl int, msg string, fields map[string]interface{}) {

	if escapeNewlines {
		msg = EscapeNewlines(msg)
	}

	if customLogger == nil {
		// Use standard logger
		if ErrLogLevel >= logLvl && loggerErr != nil {
			loggerErr.Output(depth, LogPrefix[logLvl]+msg+postFix)
		} else if LogLevel >= logLvl && logger != nil {
			logger.Output(depth, LogPrefix[logLvl]+msg+postFix)
		}
	} else {
		customLogger.Log(depth, logLvl, msg, fields)
	}
}

// DevNull Dummy discard, satisfies io.Writer without importing io or os.
// http://play.golang.org/p/5LIA41Iqfp
type DevNull struct{}

func (DevNull) Write(p []byte) (int, error) {
	return len(p), nil
}

// EscapeNewlines replaces standard newline characters with escaped newlines so long msgs will
// remain one line.
func EscapeNewlines(str string) string {
	return strings.Replace(str, "\n", "\\n", -1)
}
