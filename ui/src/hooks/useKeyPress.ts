import { useCallback, useEffect, useLayoutEffect, useRef } from 'react';

const useKeyPress = (eventConfigs: unknown, callback: unknown, node = null): void => {
  const callbackRef = useRef(callback);
  useLayoutEffect(() => {
    callbackRef.current = callback;
  });

  const handleKeyPress = useCallback(
    (event:any) => {

      // @ts-expect-error: Some?
      if (eventConfigs.some((eventConfig) => Object.keys(eventConfig).every(nameKey => eventConfig[nameKey] === event[nameKey]))) {
        event.stopPropagation()
        event.preventDefault();
        // @ts-expect-error: Some?
        callbackRef.current(event);
      }
    },
    [eventConfigs]
  );

  useEffect(() => {
    const targetNode = node || document;
    targetNode &&
      targetNode.addEventListener("keydown", handleKeyPress);

    return () =>
      targetNode &&
      targetNode.removeEventListener("keydown", handleKeyPress);
  }, [handleKeyPress, node]);
};

export default useKeyPress;
