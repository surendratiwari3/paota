package logger

import "log"

// Implementation for the default logger (can be extended with other logging libraries).

// DefaultLogger is an implementation of the Logger interface using the standard log package.
type DefaultLogger struct{}

func (l *DefaultLogger) Debug(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Info(args ...interface{}) {
	log.Println("[INFO]", args)
}

func (l *DefaultLogger) Warn(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Warning(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Error(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Fatal(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Panic(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Debugln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Infoln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Warnln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Warningln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Errorln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Fatalln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Panicln(args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Debugf(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Infof(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Warnf(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Warningf(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Errorf(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Fatalf(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}

func (l *DefaultLogger) Panicf(format string, args ...interface{}) {
	//TODO implement me
	panic("implement me")
}
