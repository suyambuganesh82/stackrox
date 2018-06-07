import { types as locationActionTypes } from '../reducers/routes';

/**
 * Redux Saga helper function that creates an action for changing the location state
 *
 * @param {string} pathname the path of the url
 * @param {string} from the previous url
 * @param {string} hash the URL hash fragment
 * @returns {Object} the action for the location state change
 *
 */
export default function createLocationChange(pathname, from, hash) {
    return {
        type: locationActionTypes.LOCATION_CHANGE,
        payload: { pathname, hash, state: { from } }
    };
}
