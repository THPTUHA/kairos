import React, { useEffect, useState } from 'react';
//@ts-ignore
import Lowlight from 'react-lowlight'
import 'highlight.js/styles/atom-one-light.css'
import styles from '../styles/index.module.sass';

//@ts-ignore
import json from 'highlight.js/lib/languages/json'
//@ts-ignore
import javascript from 'highlight.js/lib/languages/javascript'
//@ts-ignore
import yaml from 'highlight.js/lib/languages/yaml'
//@ts-ignore
import bash from 'highlight.js/lib/languages/bash'

Lowlight.registerLanguage('json', json);
Lowlight.registerLanguage('yaml', yaml);
Lowlight.registerLanguage('javascript', javascript);
Lowlight.registerLanguage('bash', bash);

interface Props {
  code: string;
  showLineNumbers?: boolean;
  language?: string;
}

export const SyntaxHighlighter: React.FC<Props> = ({
  code,
  showLineNumbers = false,
  language = null,
}) => {
  const [markers, setMarkers] = useState<any>([])

  useEffect(() => {
    const newMarkers = code.split("\n").map((item, i) => {
      return {
        line: i + 1,
        className: styles.hljsMarkerLine
      }
    });
    setMarkers(showLineNumbers ? newMarkers : []);
  }, [showLineNumbers, code])

  return <div style={{ fontSize: ".75rem" }} className={styles.highlighterContainer}><Lowlight language={language ? language : ""} value={code} markers={markers} /></div>;
};

export default SyntaxHighlighter;
