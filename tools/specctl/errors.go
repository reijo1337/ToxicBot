package main

import "errors"

// errNoCommand возвращается, когда specctl вызван без команды.
var errNoCommand = errors.New("не указана команда")

// errNoCapability возвращается, когда команде cover не передали --cap.
var errNoCapability = errors.New("не указано имя capability: specctl cover --cap <имя>")
