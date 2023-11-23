import { useState, useEffect } from 'react';

function getWindowDimensions() {
  const { innerWidth: width, innerHeight: height } = window;
  return {
    width,
    height
  };
}

interface InfoBannerTexts {
  requestText: string;
  responseText: string;
  elapsedTimeText: string;
}

export function useRequestTextByWidth(windowWidth: number): InfoBannerTexts {

  let requestText = "Request: "
  let responseText = "Response: "
  let elapsedTimeText = "Elapsed Time: "

  if (windowWidth < 1436) {
    requestText = ""
    responseText = ""
    elapsedTimeText = ""
  } else if (windowWidth < 1700) {
    requestText = "Req: "
    responseText = "Res: "
    elapsedTimeText = "ET: "
  }

  return { requestText, responseText, elapsedTimeText }
}

interface TcpStreamTexts {
  tcpStream: string;
  indexText: string;
  nodeText: string;
  tcpReplay: string;
  downloadPcap: string;
}

export function useTcpStreamTextsByWidth(windowWidth: number, layer4: string): TcpStreamTexts {

  let tcpStream = `${layer4} Stream:`
  let indexText = "Index:"
  let nodeText = "Node:"
  let tcpReplay = `${layer4} Replay`
  let downloadPcap = "Download PCAP"

  if (windowWidth < 1200) {
    tcpStream = "S:"
    indexText = "I:"
    nodeText = "N:"
    tcpReplay = "Replay"
    downloadPcap = "PCAP"
  } else if (windowWidth < 1590) {
    tcpStream = "S:"
    indexText = "I:"
    nodeText = "N:"
    tcpReplay = "Replay"
    downloadPcap = "PCAP"
  } else if (windowWidth < 1685) {
    tcpStream = "Stream:"
    tcpReplay = "Replay"
    downloadPcap = "PCAP"
  } else if (windowWidth < 1850) {
    tcpReplay = "Replay"
    downloadPcap = "PCAP"
  }

  return { tcpStream, indexText, nodeText, tcpReplay, downloadPcap }
}

export default function useWindowDimensions(): Record<string, number> {
  const [windowDimensions, setWindowDimensions] = useState(getWindowDimensions());

  useEffect(() => {
    function handleResize() {
      setWindowDimensions(getWindowDimensions());
    }

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  return windowDimensions;
}
