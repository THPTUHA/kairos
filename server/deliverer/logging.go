package deliverer

type LogLevel int

const (
	LogLevelNone LogLevel = iota
	LogLevelTrace
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type LogHandler func(LogEntry)

type logger struct {
	level   LogLevel
	handler LogHandler
}

func newLogger(level LogLevel, handler LogHandler) *logger {
	return &logger{
		level:   level,
		handler: handler,
	}
}

type LogEntry struct {
	Level   LogLevel
	Message string
	Fields  map[string]any
}

func newLogEntry(level LogLevel, message string, fields ...map[string]any) LogEntry {
	var f map[string]any
	if len(fields) > 0 {
		f = fields[0]
	}
	return LogEntry{
		Level:   level,
		Message: message,
		Fields:  f,
	}
}

func (l *logger) log(entry LogEntry) {
	if l == nil {
		return
	}
	if l.enabled(entry.Level) {
		l.handler(entry)
	}
}

func (l *logger) enabled(level LogLevel) bool {
	if l == nil {
		return false
	}
	return level >= l.level && l.level != LogLevelNone
}

func NewLogEntry(level LogLevel, message string, fields ...map[string]any) LogEntry {
	return newLogEntry(level, message, fields...)
}
