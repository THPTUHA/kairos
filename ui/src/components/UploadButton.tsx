import { MdFileUpload } from "react-icons/md";
import { parse } from "../helper/objectParser";

export const UploadButton = <T extends any>(props: { onUpload: (value: T) => void; onError: (error: Error) => void }) => {
    const handleFiles = (files: FileList) => {
        files && files[0] && files[0]
            .text()
            .then(value => props.onUpload(parse(value) as T))
            .catch(props.onError);
    };

    return (
        <label style={{ marginBottom: 2, marginRight: 2 }} className='kairos-button kairos-button--base-o' key='upload-file'>
            <input type='file' onChange={(e: any) => handleFiles(e.target.files)} style={{ display: 'none' }} />
            <span className="flex items-center">
                <MdFileUpload className="w-4 h-4" />
                <span>Upload file</span>
            </span>
        </label>
    );
};
