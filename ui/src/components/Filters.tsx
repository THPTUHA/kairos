import React, { FC, useEffect, useMemo, useRef, useState } from "react";
import { Entry } from "../models/entry";
import { useRecoilState, useRecoilValue } from "recoil";
import queryBackgroundColorAtom from "../recoil/queryBackgroundColor/atom";
import queryBuildAtom from "../recoil/queryBuild/atom";
import { Button, debounce } from "@mui/material";
import { ColorWhite } from "../conts";
import CodeEditor from '@uiw/react-textarea-code-editor';
import { Grid } from "@mui/material";
import { useLocation, useNavigate } from "react-router-dom";
import queryAtom from "../recoil/query/atom";
import shortcutsKeyboard from "../configs/shortcutsKeyboard";
import useKeyPress from "../hooks/useKeyPress";

interface CodeEditorWrap {
    reopenConnection?: () => void;
    onQueryChange?: (q: string) => void
    onValidationChanged?: (event: OnQueryChange) => void
  }
  
  interface QueryFormProps extends CodeEditorWrap {
    entries: Entry[];
  }
  
  export const Filters: React.FC<QueryFormProps> = ({ entries, reopenConnection, onQueryChange }) => {
    return <div className="">
      <QueryForm
        entries={entries}
        reopenConnection={reopenConnection}
        onQueryChange={onQueryChange}
      />
    </div>;
  };
  
  type OnQueryChange = { valid: boolean, message: string, query: string }
  
  export const modalStyle = {
    position: 'absolute',
    top: '10%',
    left: '50%',
    transform: 'translate(-50%, 0%)',
    width: '80vw',
    bgcolor: 'background.paper',
    borderRadius: '5px',
    boxShadow: 24,
    outline: "none",
    p: 4,
    color: '#000',
  };
  
  export const CodeEditorWrap: FC<CodeEditorWrap> = ({ onQueryChange, onValidationChanged }) => {
    const [queryBackgroundColor, setQueryBackgroundColor] = useRecoilState(queryBackgroundColorAtom);
  
    const queryBuild = useRecoilValue(queryBuildAtom);
  
    const handleQueryChange = useMemo(
      () =>
        debounce(async (query: string) => {
          if (!query) {
            setQueryBackgroundColor(ColorWhite);
            onValidationChanged && onValidationChanged({ query: query, message: "", valid: true });
          } else {
            console.log({query})
            // fetch(`${HubBaseUrl}/query/validate?q=${encodeURIComponent(query)}`)
            //   .then(response => response.ok ? response : response.text().then(err => Promise.reject(err)))
            //   .then(response => response.json())
            //   .then(data => {
            //     if (data.valid) {
            //       setQueryBackgroundColor(ColorGreen);
            //     } else {
            //       setQueryBackgroundColor(ColorRed);
            //     }
            //     onValidationChanged && onValidationChanged({ query: query, message: data.message, valid: data.valid })
            //   })
            //   .catch(err => {
            //     console.error(err);
            //     toast.error(err.toString(), {
            //       theme: "colored"
            //     });
            //   });
          }
        }, 100),
      [onValidationChanged]
    ) as (query: string) => void;
  
    useEffect(() => {
      handleQueryChange(queryBuild);
    }, [queryBuild, handleQueryChange]);
  
    return <CodeEditor
      value={queryBuild}
      language="py"
      placeholder="Kairos Filter Syntax"
      onChange={(event) => onQueryChange ? onQueryChange(event.target.value):""}
      padding={8}
      style={{
        fontSize: 14,
        backgroundColor: `${queryBackgroundColor}`,
        fontFamily: 'ui-monospace,SFMono-Regular,SF Mono,Consolas,Liberation Mono,Menlo,monospace',
      }}
    />
  }
  
  export const QueryForm: React.FC<QueryFormProps> = ({ entries, reopenConnection, onQueryChange, onValidationChanged }) => {
  
    const formRef = useRef<HTMLFormElement>(null);
  
    const queryBuild = useRecoilValue(queryBuildAtom);
    const [query, setQuery] = useRecoilState(queryAtom);
  
  
    const navigate = useNavigate();
    const location = useLocation();
  
    const handleSubmit = (e:any) => {
      setQuery(queryBuild);
      navigate({ pathname: location.pathname, search: `q=${encodeURIComponent(queryBuild)}` });
      e.preventDefault();
    }
    //@ts-ignore
    useKeyPress(shortcutsKeyboard.ctrlEnter, handleSubmit, formRef.current);
  
    return <React.Fragment>
      <form
        ref={formRef}
        onSubmit={handleSubmit}
        style={{
          width: '100%',
        }}
      >
        <Grid container spacing={2}>
          <Grid
            item
            xs={8}
            style={{
              maxHeight: '25vh',
              overflowY: 'auto',
            }}
          >
            <label>
              <CodeEditorWrap onQueryChange={onQueryChange} onValidationChanged={onValidationChanged} />
            </label>
          </Grid>
          <Grid item xs={4}>
            <Button
              type="submit"
              variant="contained"
              className=""
            >
              Apply
            </Button>
          </Grid>
        </Grid>
      </form>
  
    </React.Fragment>
  }
  