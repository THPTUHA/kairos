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

interface EntrySummaryProps {
    entry: Entry;
}

const EntrySummary: React.FC<EntrySummaryProps> = ({ entry }) => {
    return <div>
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
            label: 'Payload',
            children:  <ReactJson 
                            src={focusedItem?.payload && typeof focusedItem?.payload == "object"
                            ? focusedItem?.payload: {} } name={false} 
                            enableClipboard={false}
                        />,
        },
    ];

    return <LoadingWrapper isLoading={isLoading} loaderMargin={50} loaderHeight={60}>
        {focusedItem && <React.Fragment>
            <EntrySummary entry={focusedItem} />
            <Tabs defaultActiveKey="1" items={items} />
        </React.Fragment>}
    </LoadingWrapper>
};