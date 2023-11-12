import { parse } from "../helper/objectParser";

export const UploadButton = <T extends any>(props: {onUpload: (value: T) => void; onError: (error: Error) => void}) => {
    const handleFiles = (files: FileList) => {
        console.log(files[0])
        files[0]
            .text()
            .then(value => props.onUpload(parse(value) as T))
            .catch(props.onError);
    };

    return (
        <label style={{marginBottom: 2, marginRight: 2}} className='kairos-button kairos-button--base-o' key='upload-file'>
            <input type='file' onChange={(e:any) => handleFiles(e.target.files)} style={{display: 'none'}} />
            <i className='fa fa-upload' /> Upload file
        </label>
    );
};
