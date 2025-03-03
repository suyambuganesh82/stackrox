import React, { useMemo, useState } from 'react';
import { Alert, Button, debounce, Flex, SearchInput } from '@patternfly/react-core';

import BacklogListSelector, {
    BacklogListSelectorProps,
} from 'Components/PatternFly/BacklogListSelector';
import { Collection } from 'services/CollectionsService';
import useEmbeddedCollections from './hooks/useEmbeddedCollections';

export type CollectionAttacherProps = {
    // A collection ID that should not be visible in the collection attacher component. This is
    // used when editing a collection to prevent reference cycles.
    excludedCollectionId: string | null;
    initialEmbeddedCollections: Collection[];
    onSelectionChange: (collections: Collection[]) => void;
    collectionTableCells: BacklogListSelectorProps<Collection>['cells'];
};

function compareNameLowercase(search: string): (item: { name: string }) => boolean {
    return ({ name }) => name.toLowerCase().includes(search.toLowerCase());
}

function CollectionAttacher({
    excludedCollectionId,
    initialEmbeddedCollections,
    onSelectionChange,
    collectionTableCells,
}: CollectionAttacherProps) {
    const [search, setSearch] = useState('');
    const embedded = useEmbeddedCollections(excludedCollectionId, initialEmbeddedCollections);
    const { attached, detached, attach, detach, hasMore, fetchMore, onSearch } = embedded;
    const { isFetchingMore, fetchMoreError } = embedded;

    const onSearchInputChange = useMemo(
        () =>
            debounce((value: string) => {
                setSearch(value);
                onSearch(value);
            }, 800),
        [onSearch]
    );

    const selectedOptions = attached.filter(compareNameLowercase(search));
    const deselectedOptions = detached.filter(compareNameLowercase(search));

    return (
        <Flex direction={{ default: 'column' }} spaceItems={{ default: 'spaceItemsXl' }}>
            <SearchInput
                aria-label="Filter by name"
                placeholder="Filter by name"
                value={search}
                onChange={onSearchInputChange}
            />
            <BacklogListSelector
                selectedOptions={selectedOptions}
                deselectedOptions={deselectedOptions}
                onSelectItem={({ id }) => attach(id)}
                onDeselectItem={({ id }) => detach(id)}
                onSelectionChange={onSelectionChange}
                rowKey={({ id }) => id}
                cells={collectionTableCells}
                selectedLabel="Attached collections"
                deselectedLabel="Available collections"
                selectButtonText="Attach"
                deselectButtonText="Detach"
            />
            {fetchMoreError && (
                <Alert
                    variant="danger"
                    isInline
                    title="There was an error loading more collections"
                />
            )}
            {hasMore && (
                <Button
                    className="pf-u-align-self-flex-start"
                    variant="secondary"
                    onClick={() => fetchMore(search)}
                    isLoading={isFetchingMore}
                >
                    View more
                </Button>
            )}
        </Flex>
    );
}

export default CollectionAttacher;
