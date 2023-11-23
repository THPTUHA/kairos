import * as React from 'react';
import { createRef, useEffect, useState } from 'react';
import MonacoEditor from 'react-monaco-editor';
import { ScopedLocalStorage } from './ScopedLocalStorage';
import { parse, stringify } from '../helper/objectParser';
import { PhaseIcon } from './PhaseIcon';
import { AiFillCaretRight } from 'react-icons/ai'

interface Props<T> {
    value: T;
    buttons?: React.ReactNode;
    onChange?: (value: T) => void;
}

const defaultLang = 'yaml';

export const ObjectEditor = <T extends any>({ value, buttons, onChange }: Props<T>) => {
    const storage = new ScopedLocalStorage('object-editor');
    const [error, setError] = useState<Error>();
    const [lang, setLang] = useState<string>(defaultLang);
    const [text, setText] = useState<string>(stringify(value, lang));

    useEffect(() => storage.setItem('lang', lang, defaultLang), [lang]);
    useEffect(() => setText(stringify(value, lang)), [value]);
    useEffect(() => setText(stringify(parse(text), lang)), [lang]);
    useEffect(() => {
        // @ts-ignore
        const editorText = stringify(parse(editor.current.editor.getValue()), lang);
        // @ts-ignore
        const editorLang = editor.current.editor.getValue().startsWith('{') ? 'json' : 'yaml';
        if (text !== editorText || lang !== editorLang) {
            // @ts-ignore
            editor.current.editor.setValue(stringify(parse(text), lang));
        }
    }, [text, lang]);

    const editor = createRef<MonacoEditor>();
    // this calculation is rough, it is probably hard to work for for every case, essentially it is:
    // some pixels above and below for buttons, plus a bit of a buffer/padding
    const height = Math.max(600, window.innerHeight * 0.9 - 250);

    return (
        <>
            <div style={{ paddingBottom: '1em' }} className='flex w-full'>
                {
                    // @ts-ignore
                    Object.keys(value).map(x => (
                        <button
                            key={x}
                            className='rounded mx-2 px-2'
                            onClick={() => {
                                const index = text.split('\n').findIndex((y, i) => (lang === 'yaml' ? y.startsWith(x + ':') : y.includes('"' + x + '":')));

                                if (index >= 0) {
                                    const lineNumber = index + 1;
                                    // @ts-ignore
                                    editor.current.editor.revealLineInCenter(lineNumber);
                                    // @ts-ignore
                                    editor.current.editor.setPosition({ lineNumber, column: 0 });
                                    // @ts-ignore
                                    editor.current.editor.focus();
                                }
                            }}>
                            <div className='flex items-center'>
                                <AiFillCaretRight />
                                <p>{x}</p>
                            </div>
                        </button>
                    ))}
                {buttons}
            </div>
            <div>
                <MonacoEditor
                    ref={editor}
                    key='editor'
                    defaultValue={text}
                    language={lang}
                    height={height + 'px'}
                    options={{
                        readOnly: onChange ? false : true,
                        minimap: { enabled: false },
                        lineNumbers: 'on',
                        guides: {
                            indentation: false
                        },
                        scrollBeyondLastLine: true
                    }}
                    onChange={v => {
                        if (onChange) {
                            try {
                                onChange(parse(v));
                                // @ts-ignore
                                setError(null);
                            } catch (e) {
                                // @ts-ignore
                                setError(e);
                            }
                        }
                    }}
                />
            </div>
            {error && (
                <div style={{ paddingTop: '1em' }}>
                    <PhaseIcon value='Error' /> {error.message}
                </div>
            )}
        </>
    );
};
