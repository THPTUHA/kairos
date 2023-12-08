import { makeStyles } from "@mui/styles";
import { Entry } from "../../models/entry";
import useWindowDimensions, { useRequestTextByWidth } from "../../hooks/WindowDimensionsHook";
import Queryable from "../Queryable";
import { EntryItem } from "./EntryListItem";
import { useRecoilState, useRecoilValue } from "recoil";
import focusedItemAtom from "../../recoil/focusedItem/atom";
import focusedContextAtom from "../../recoil/focusedContext/atom";
import queryAtom from "../../recoil/query/atom";
import React, { useEffect, useState } from "react";
import entryDataAtom from "../../recoil/entryData";
import { LoadingWrapper } from "../LoadingWrapper";
import { Tabs, TabsProps } from "antd";
import { Utils } from "../../helper/utils";
import ReactJson from 'react-json-view'


const useStyles = makeStyles(() => ({
    entryTitle: {
        display: 'flex',
        minHeight: 20,
        maxHeight: 46,
        alignItems: 'center',
        marginBottom: 4,
        marginLeft: 6,
        padding: 2,
        paddingBottom: 0
    },
    entrySummary: {
        display: 'flex',
        minHeight: 36,
        maxHeight: 46,
        alignItems: 'center',
        marginBottom: 4,
        padding: 5,
        paddingBottom: 0
    }
}));


interface EntryTitleProps {
    entry: Entry
}

export const formatSize = (n: number): string => n > 1000 ? `${Math.round(n / 1000)}kB` : `${n}B`;
const minSizeDisplayRequestSize = 880;
const EntryTitle: React.FC<EntryTitleProps> = ({ entry }) => {
    const classes = useStyles();
    const request = entry.request;
    const response = entry.response;

    const { width } = useWindowDimensions();
    const { requestText, responseText, elapsedTimeText } = useRequestTextByWidth(width)

    return <div className={classes.entryTitle}>
        {(width > minSizeDisplayRequestSize) && <div style={{ right: "30px", position: "absolute", display: "flex" }}>
            <>
                {request && <Queryable
                    query={`requestSize == ${entry.requestSize}`}
                    style={{ margin: "0 18px" }}
                    displayIconOnMouseOver={true}
                >
                    <div
                        style={{ opacity: 0.5 }}
                        id="entryDetailedTitleRequestSize"
                    >
                        {`${requestText}${formatSize(entry.requestSize)}`}
                    </div>
                </Queryable>}
                {response && <Queryable
                    query={`responseSize == ${entry.responseSize}`}
                    style={{ margin: "0 18px" }}
                    displayIconOnMouseOver={true}
                >
                    <div
                        style={{ opacity: 0.5 }}
                        id="entryDetailedTitleResponseSize"
                    >
                        {`${responseText}${formatSize(entry.responseSize)}`}
                    </div>
                </Queryable>}
                {response && <div
                    style={{ opacity: 0.5 }}
                    id="entryDetailedTitleElapsedTime"
                >
                    {`${elapsedTimeText}${Math.round(entry.elapsedTime)}ms`}
                </div>}
            </>
        </div>}
    </div>;
};

interface EntrySummaryProps {
    entry: Entry;
}

const EntrySummary: React.FC<EntrySummaryProps> = ({ entry }) => {
    return <div>
        <div className="flex w-2/3 justify-between">
            <span>Request: {Utils.humanReadableBytes(entry.requestSize)}</span>
            <span>Response:{Utils.humanReadableBytes(entry.responseSize)}</span>
            <span>Elapsed Time: {entry.elapsedTime}ms</span>
        </div>
        <EntryItem
            key={entry.id}
            entry={entry}
            style={{}}
            headingMode={true}
        />
    </div>
};

export const EntryDetailed: React.FC = () => {
    const focusedItem = useRecoilValue(focusedItemAtom);
    const query = useRecoilValue(queryAtom);
    const [isLoading, setIsLoading] = useState(false);
    const items: TabsProps['items'] = [
        {
            key: '1',
            label: 'Request',
            children:  <ReactJson src={focusedItem?.request ? focusedItem?.request: {} } />,
        },
        {
            key: '2',
            label: 'Response',
            children: <ReactJson src={focusedItem?.response ?focusedItem?.response : {}} />,
        },
    ];

    return <LoadingWrapper isLoading={isLoading} loaderMargin={50} loaderHeight={60}>
        {focusedItem && <React.Fragment>
            <EntrySummary entry={focusedItem} />
            <Tabs defaultActiveKey="1" items={items} />
        </React.Fragment>}
    </LoadingWrapper>
};