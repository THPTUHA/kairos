import React, { useMemo, useState } from "react";
import { Entry } from "../../models/entry";
import ScrollableFeedVirtualized from "react-scrollable-feed-virtualized";
import { useInterval } from "../../helper/interval";
import styles from "../../styles/EntriesList.module.sass";
import { EntryItem } from "./EntryListItem";
import down from "../../assets/downImg.svg";

interface EntriesListProps {
    entries: Entry[];
    listEntryREF: React.LegacyRef<HTMLDivElement>;
    onSnapBrokenEvent: () => void;
    isSnappedToBottom: boolean;
    setIsSnappedToBottom: (state: boolean) => void;
    scrollableRef: React.MutableRefObject<ScrollableFeedVirtualized>;
    setOffset: any
}

export const EntriesList: React.FC<EntriesListProps> = ({
    entries,
    listEntryREF,
    onSnapBrokenEvent,
    scrollableRef,
    setOffset
}) => {
    const [timeNow, setTimeNow] = useState(new Date());

    useInterval(async () => {
        setTimeNow(new Date());
    }, 1000, true);

    const memoizedEntries = useMemo(() => {
        return entries;
    }, [entries]);

    return <React.Fragment>
        <div className={styles.list}>
            <div id="list" ref={listEntryREF} className={`${styles.list}`}>
                {
                    //@ts-ignore
                    <ScrollableFeedVirtualized ref={scrollableRef} itemHeight={48} marginTop={10} onSnapBroken={onSnapBrokenEvent}>
                        {false /* It's because the first child is ignored by ScrollableFeedVirtualized */}
                        {memoizedEntries.map(entry => {
                            return <EntryItem
                                key={entry.id}
                                entry={entry}
                                style={{}}
                                headingMode={false}
                            />
                        })}
                    </ScrollableFeedVirtualized>
                }
            </div>

            <div className={styles.footer}>
                <div>Showing <b id="item-count">{entries.length}</b> items
                </div>
                <div onClick={() => {
                    setOffset((o: any) => {
                        if (o) {
                            return o - 1
                        }
                        return o
                    })
                }}
                className="cursor-pointer"
                >{"<< Pre"}</div>
                <div onClick={() => {
                    if (entries.length > 0) {
                        setOffset((o: any) => o + 1)
                    }
                }}
                className="cursor-pointer"
                >{"Next >>"}</div>

            </div>
            <div className="mx-2"></div>
        </div>
    </React.Fragment>;
};
