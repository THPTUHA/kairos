import classNames from 'classnames';
import { Utils } from '../helper/utils';

export const PhaseIcon = ({value}: {value: string}) => {
    return <i className={classNames('fa', Utils.statusIconClasses(value))} aria-hidden='true' />;
};
